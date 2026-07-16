package main

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/BullionBear/seq/core/model/command"
	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/core/msgbus"
)

// maxPayloadLen guards against corrupt records claiming absurd payload sizes.
const maxPayloadLen = 64 << 20 // 64 MB

// jsonRecord is the output structure for each line in the .jsonl file.
type jsonRecord struct {
	// For events
	Topic   string `json:"topic,omitempty"`
	EventID uint64 `json:"event_id,omitempty"`

	// For commands
	CommandType string `json:"command_type,omitempty"`
	CommandID   uint64 `json:"command_id,omitempty"`

	CreatedAt uint64 `json:"created_at"`
	Data      any    `json:"data"`
}

func main() {
	inputPath := flag.String("i", "", "Input .dat file path (required)")
	outputPath := flag.String("o", "", "Output .jsonl file path (default: stdout)")
	flag.Parse()

	if *inputPath == "" {
		fmt.Fprintln(os.Stderr, "Usage: parser -i <input.dat> [-o <output.jsonl>]")
		os.Exit(1)
	}

	inFile, err := os.Open(*inputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer inFile.Close()

	var out *bufio.Writer
	if *outputPath != "" {
		outFile, err := os.Create(*outputPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		defer outFile.Close()
		out = bufio.NewWriter(outFile)
	} else {
		out = bufio.NewWriter(os.Stdout)
	}

	if err := parseFile(inFile, out); err != nil {
		out.Flush()
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	out.Flush()
}

// parseFile validates the .dat file header and converts every record to a
// JSON line on w. Unversioned (pre-schema) files are refused: they predate
// the format contract and cannot be decoded reliably.
func parseFile(r io.Reader, w io.Writer) error {
	reader := bufio.NewReader(r)

	fileHdr := make([]byte, msgbus.FileHeaderSize)
	if _, err := io.ReadFull(reader, fileHdr); err != nil {
		return fmt.Errorf("reading file header: %w", err)
	}
	hdr, err := msgbus.DecodeFileHeader(fileHdr)
	if err != nil {
		return err
	}
	isEvent := hdr.StreamType == msgbus.StreamTypeEvent

	headerBuf := make([]byte, msgbus.RecordHeaderSize)
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)

	for recordIdx := 0; ; recordIdx++ {
		if _, err := io.ReadFull(reader, headerBuf); err != nil {
			if err == io.EOF {
				return nil
			}
			if err == io.ErrUnexpectedEOF {
				return fmt.Errorf("record %d: truncated record header", recordIdx)
			}
			return fmt.Errorf("record %d: reading header: %w", recordIdx, err)
		}

		schemaVersion := binary.LittleEndian.Uint16(headerBuf[0:])
		msgType := binary.LittleEndian.Uint16(headerBuf[2:])
		payloadLen := binary.LittleEndian.Uint32(headerBuf[4:])
		seqID := binary.LittleEndian.Uint64(headerBuf[8:])
		createdAt := binary.LittleEndian.Uint64(headerBuf[16:])

		if schemaVersion != msgbus.SchemaVersion {
			return fmt.Errorf("record %d: unsupported record schema version %d (reader supports v%d)",
				recordIdx, schemaVersion, msgbus.SchemaVersion)
		}
		if payloadLen > maxPayloadLen {
			return fmt.Errorf("record %d: implausible payload length %d", recordIdx, payloadLen)
		}

		payload := make([]byte, payloadLen)
		if payloadLen > 0 {
			if _, err := io.ReadFull(reader, payload); err != nil {
				return fmt.Errorf("record %d: reading payload: %w", recordIdx, err)
			}
		}

		var rec jsonRecord
		rec.CreatedAt = createdAt

		if isEvent {
			topic := event.Topic(msgType)
			rec.Topic = topic.String()
			rec.EventID = seqID
			rec.Data = decodeEventPayload(topic, payload)
		} else {
			cmdType := command.CommandType(msgType)
			rec.CommandType = cmdType.String()
			rec.CommandID = seqID
			rec.Data = decodeCommandPayload(cmdType, payload)
		}

		if err := encoder.Encode(rec); err != nil {
			return fmt.Errorf("record %d: encoding JSON: %w", recordIdx, err)
		}
	}
}

// orDecodeErr adapts the (T, error) decoder signature to the any-typed JSON
// output: decode failures are reported inline in the record instead of
// aborting the whole file.
func orDecodeErr[T any](v T, err error) any {
	if err != nil {
		return map[string]any{"decode_error": err.Error()}
	}
	return v
}

func decodeEventPayload(topic event.Topic, buf []byte) any {
	if len(buf) == 0 {
		return nil
	}
	switch topic {
	// State events
	case event.TopicEventAbnormal:
		return orDecodeErr(event.NewAbnormalEventFromBytes(buf))
	case event.TopicEventReady:
		return orDecodeErr(event.NewReadyEventFromBytes(buf))
	case event.TopicEventStop:
		return orDecodeErr(event.NewStopEventFromBytes(buf))
	case event.TopicEventFinished:
		return orDecodeErr(event.NewFinishedEventFromBytes(buf))

	// Market data
	case event.TopicEventDepthSnapshot:
		return orDecodeErr(event.NewDepthSnapshotFromBytes(buf))
	case event.TopicEventRespDepthSnapshot:
		return orDecodeErr(event.NewRespDepthSnapshotFromBytes(buf))
	case event.TopicEventDepthUpdate:
		return orDecodeErr(event.NewDepthUpdateFromBytes(buf))
	case event.TopicEventTick:
		return orDecodeErr(event.NewTickFromBytes(buf))

	// Timer
	case event.TopicEventTimer:
		return orDecodeErr(event.NewTimeEventFromBytes(buf))

	// Execution data
	case event.TopicEventOrderNew:
		return orDecodeErr(event.NewOrderNewFromBytes(buf))
	case event.TopicEventOrderUnknownStatus:
		return orDecodeErr(event.NewOrderUnknownStatusFromBytes(buf))
	case event.TopicEventOrderError:
		return orDecodeErr(event.NewOrderErrorFromBytes(buf))
	case event.TopicEventOrderRiskInvalid:
		return orDecodeErr(event.NewOrderRiskInvalidFromBytes(buf))
	case event.TopicEventOrderAccepted:
		return orDecodeErr(event.NewOrderAcceptedFromBytes(buf))
	case event.TopicEventOrderPartialFill:
		return orDecodeErr(event.NewOrderPartiallyFilledFromBytes(buf))
	case event.TopicEventOrderFilled:
		return orDecodeErr(event.NewOrderFilledFromBytes(buf))
	case event.TopicEventExecution:
		return orDecodeErr(event.NewExecutionFromBytes(buf))
	case event.TopicEventOrderCanceled:
		return orDecodeErr(event.NewOrderCanceledFromBytes(buf))
	case event.TopicEventOrderRejected:
		return orDecodeErr(event.NewOrderRejectedFromBytes(buf))

	// Reconciliation data
	case event.TopicEventRespBalanceSnapshot:
		return orDecodeErr(event.NewRespBalanceSnapshotFromBytes(buf))
	case event.TopicEventBalanceUpdate:
		return orDecodeErr(event.NewBalanceUpdateFromBytes(buf))

	default:
		return map[string]any{"raw_len": len(buf)}
	}
}

func decodeCommandPayload(cmdType command.CommandType, buf []byte) any {
	if len(buf) == 0 {
		return nil
	}
	switch cmdType {
	case command.CommandTypeOrderRiskCheck:
		return orDecodeErr(command.NewRiskCheckFromBytes(buf))
	case command.CommandTypeOrderSubmit:
		return orDecodeErr(command.NewSubmitOrderFromBytes(buf))
	case command.CommandTypeOrderCancel:
		return orDecodeErr(command.NewCancelOrderFromBytes(buf))
	case command.CommandTypeCancelAll:
		return orDecodeErr(command.NewCancelAllFromBytes(buf))
	case command.CommandTypeQryBalanceSnapshot:
		return orDecodeErr(command.NewQryBalanceSnapshotFromBytes(buf))
	case command.CommandTypeReqDepthSnapshot:
		return orDecodeErr(command.NewReqDepthSnapshotFromBytes(buf))
	default:
		return map[string]any{"raw_len": len(buf)}
	}
}
