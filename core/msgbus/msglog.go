package msgbus

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/BullionBear/seq/core/model/command"
	"github.com/BullionBear/seq/core/model/event"
)

// jsonRecord is one line in an event_*.jsonl / command_*.jsonl file.
type jsonRecord struct {
	Topic       string `json:"topic,omitempty"`
	EventID     uint64 `json:"event_id,omitempty"`
	CommandType string `json:"command_type,omitempty"`
	CommandID   uint64 `json:"command_id,omitempty"`
	CreatedAt   uint64 `json:"created_at"`
	Data        any    `json:"data"`
}

// MsgLogger writes plaintext JSONL event/command records to date-stamped
// .jsonl files. It is designed to be called from the single dispatch
// goroutine, so no locking is required.
type MsgLogger struct {
	dir string

	eventFile   *os.File
	eventWriter *bufio.Writer
	eventEnc    *json.Encoder
	eventDate   string

	commandFile   *os.File
	commandWriter *bufio.Writer
	commandEnc    *json.Encoder
	commandDate   string
}

// NewMsgLogger creates a new MsgLogger that writes to the given directory.
// The directory is created if it does not exist.
func NewMsgLogger(dir string) (*MsgLogger, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("msglog: mkdir %s: %w", dir, err)
	}
	return &MsgLogger{dir: dir}, nil
}

// LogEvent writes a JSONL record for an event to the event .jsonl file.
func (l *MsgLogger) LogEvent(ev Event, payload []byte) {
	today := time.Now().UTC().Format("2006-01-02")
	if l.eventEnc == nil || today != l.eventDate {
		l.rotateEventFile(today)
	}
	if l.eventEnc == nil {
		return
	}
	rec := jsonRecord{
		Topic:     ev.Ref.Topic.String(),
		EventID:   ev.EventID,
		CreatedAt: ev.CreatedAt,
		Data:      decodeEventPayload(ev.Ref.Topic, payload),
	}
	if err := l.eventEnc.Encode(rec); err != nil {
		log().Error().Err(err).Msg("msglog: failed to encode event record")
	}
}

// LogCommand writes a JSONL record for a command to the command .jsonl file.
func (l *MsgLogger) LogCommand(cmd Command, payload []byte) {
	today := time.Now().UTC().Format("2006-01-02")
	if l.commandEnc == nil || today != l.commandDate {
		l.rotateCommandFile(today)
	}
	if l.commandEnc == nil {
		return
	}
	rec := jsonRecord{
		CommandType: cmd.Ref.CommandType.String(),
		CommandID:   cmd.CommandID,
		CreatedAt:   cmd.CreatedAt,
		Data:        decodeCommandPayload(cmd.Ref.CommandType, payload),
	}
	if err := l.commandEnc.Encode(rec); err != nil {
		log().Error().Err(err).Msg("msglog: failed to encode command record")
	}
}

// Close flushes and closes all open files.
func (l *MsgLogger) Close() {
	if l.eventWriter != nil {
		l.eventWriter.Flush()
	}
	if l.eventFile != nil {
		l.eventFile.Close()
	}
	if l.commandWriter != nil {
		l.commandWriter.Flush()
	}
	if l.commandFile != nil {
		l.commandFile.Close()
	}
}

func (l *MsgLogger) rotateEventFile(date string) {
	if l.eventWriter != nil {
		l.eventWriter.Flush()
	}
	if l.eventFile != nil {
		l.eventFile.Close()
	}
	path := filepath.Join(l.dir, fmt.Sprintf("event_%s.jsonl", date))
	f, enc, w, err := openJSONL(path)
	if err != nil {
		log().Error().Err(err).Str("path", path).Msg("msglog: failed to open event file")
		l.eventFile = nil
		l.eventWriter = nil
		l.eventEnc = nil
		return
	}
	l.eventFile = f
	l.eventWriter = w
	l.eventEnc = enc
	l.eventDate = date
}

func (l *MsgLogger) rotateCommandFile(date string) {
	if l.commandWriter != nil {
		l.commandWriter.Flush()
	}
	if l.commandFile != nil {
		l.commandFile.Close()
	}
	path := filepath.Join(l.dir, fmt.Sprintf("command_%s.jsonl", date))
	f, enc, w, err := openJSONL(path)
	if err != nil {
		log().Error().Err(err).Str("path", path).Msg("msglog: failed to open command file")
		l.commandFile = nil
		l.commandWriter = nil
		l.commandEnc = nil
		return
	}
	l.commandFile = f
	l.commandWriter = w
	l.commandEnc = enc
	l.commandDate = date
}

func openJSONL(path string) (*os.File, *json.Encoder, *bufio.Writer, error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, nil, nil, err
	}
	w := bufio.NewWriter(f)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return f, enc, w, nil
}

// orDecodeErr adapts the (T, error) decoder signature to the any-typed JSON
// output: decode failures are reported inline in the record instead of
// aborting the write.
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
	case event.TopicEventAbnormal:
		return orDecodeErr(event.NewAbnormalEventFromBytes(buf))
	case event.TopicEventReady:
		return orDecodeErr(event.NewReadyEventFromBytes(buf))
	case event.TopicEventStop:
		return orDecodeErr(event.NewStopEventFromBytes(buf))
	case event.TopicEventFinished:
		return orDecodeErr(event.NewFinishedEventFromBytes(buf))
	case event.TopicEventDepthSnapshot:
		return orDecodeErr(event.NewDepthSnapshotFromBytes(buf))
	case event.TopicEventRespDepthSnapshot:
		return orDecodeErr(event.NewRespDepthSnapshotFromBytes(buf))
	case event.TopicEventDepthUpdate:
		return orDecodeErr(event.NewDepthUpdateFromBytes(buf))
	case event.TopicEventTick:
		return orDecodeErr(event.NewTickFromBytes(buf))
	case event.TopicEventTimer:
		return orDecodeErr(event.NewTimeEventFromBytes(buf))
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
