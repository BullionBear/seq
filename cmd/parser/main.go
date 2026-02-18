package main

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/BullionBear/seq/core/model/command"
	"github.com/BullionBear/seq/core/model/event"
	"github.com/BullionBear/seq/core/msgbus"
)

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

	// Detect file type from filename
	base := filepath.Base(*inputPath)
	isEvent := strings.HasPrefix(base, "event_")
	isCommand := strings.HasPrefix(base, "command_")
	if !isEvent && !isCommand {
		fmt.Fprintf(os.Stderr, "Error: filename must start with 'event_' or 'command_' (got %q)\n", base)
		os.Exit(1)
	}

	// Open input
	inFile, err := os.Open(*inputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer inFile.Close()

	// Open output
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
	defer out.Flush()

	reader := bufio.NewReader(inFile)
	headerBuf := make([]byte, msgbus.RecordHeaderSize)
	encoder := json.NewEncoder(out)
	encoder.SetEscapeHTML(false)

	for {
		// Read header
		if _, err := io.ReadFull(reader, headerBuf); err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				break
			}
			fmt.Fprintf(os.Stderr, "Error reading header: %v\n", err)
			os.Exit(1)
		}

		topicOrType := int32(binary.LittleEndian.Uint32(headerBuf[0:]))
		seqID := binary.LittleEndian.Uint64(headerBuf[4:])
		createdAt := binary.LittleEndian.Uint64(headerBuf[12:])
		payloadLen := binary.LittleEndian.Uint32(headerBuf[20:])

		// Read payload
		payload := make([]byte, payloadLen)
		if payloadLen > 0 {
			if _, err := io.ReadFull(reader, payload); err != nil {
				fmt.Fprintf(os.Stderr, "Error reading payload: %v\n", err)
				os.Exit(1)
			}
		}

		var rec jsonRecord
		rec.CreatedAt = createdAt

		if isEvent {
			topic := event.Topic(topicOrType)
			rec.Topic = topic.String()
			rec.EventID = seqID
			rec.Data = decodeEventPayload(topic, payload)
		} else {
			cmdType := command.CommandType(topicOrType)
			rec.CommandType = cmdType.String()
			rec.CommandID = seqID
			rec.Data = decodeCommandPayload(cmdType, payload)
		}

		if err := encoder.Encode(rec); err != nil {
			fmt.Fprintf(os.Stderr, "Error encoding JSON: %v\n", err)
			os.Exit(1)
		}
	}
}

func decodeEventPayload(topic event.Topic, buf []byte) any {
	if len(buf) == 0 {
		return nil
	}
	switch topic {
	// State events
	case event.TopicEventAbnormal:
		return event.NewAbnormalEventFromBytes(buf)
	case event.TopicEventReady:
		return event.NewReadyEventFromBytes(buf)
	case event.TopicEventStop:
		return event.NewStopEventFromBytes(buf)
	case event.TopicEventFinished:
		return event.NewFinishedEventFromBytes(buf)

	// Market data
	case event.TopicEventDepthSnapshot:
		return event.NewDepthSnapshotFromBytes(buf)
	case event.TopicEventRespDepthSnapshot:
		return event.NewRespDepthSnapshotFromBytes(buf)
	case event.TopicEventDepthUpdate:
		return event.NewDepthUpdateFromBytes(buf)
	case event.TopicEventTick:
		return event.NewTickFromBytes(buf)

	// Execution data
	case event.TopicEventOrderUnknownStatus:
		return event.NewOrderUnknownStatusFromBytes(buf)
	case event.TopicEventOrderError:
		return event.NewOrderErrorFromBytes(buf)
	case event.TopicEventOrderRiskInvalid:
		return event.NewOrderRiskInvalidFromBytes(buf)
	case event.TopicEventOrderAccepted:
		return event.NewOrderAcceptedFromBytes(buf)
	case event.TopicEventOrderPartialFill:
		return event.NewOrderPartiallyFilledFromBytes(buf)
	case event.TopicEventOrderFill:
		return event.NewFillFromBytes(buf)
	case event.TopicEventOrderCanceled:
		return event.NewOrderCanceledFromBytes(buf)
	case event.TopicEventOrderRejected:
		return event.NewOrderRejectedFromBytes(buf)

	// Reconciliation data
	case event.TopicEventRespBalanceSnapshot:
		return event.NewRespBalanceSnapshotFromBytes(buf)
	case event.TopicEventBalanceUpdate:
		return event.NewBalanceUpdateFromBytes(buf)

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
		return command.NewRiskCheckFromBytes(buf)
	case command.CommandTypeOrderSubmit:
		return command.NewSubmitOrderFromBytes(buf)
	case command.CommandTypeOrderCancel:
		return command.NewCancelOrderFromBytes(buf)
	case command.CommandTypeCancelAll:
		return command.NewCancelAllFromBytes(buf)
	case command.CommandTypeQryBalanceSnapshot:
		return command.NewQryBalanceSnapshotFromBytes(buf)
	case command.CommandTypeReqDepthSnapshot:
		return command.NewReqDepthSnapshotFromBytes(buf)
	default:
		return map[string]any{"raw_len": len(buf)}
	}
}
