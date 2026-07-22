package msgbus

import (
	"fmt"
	"strconv"

	"github.com/BullionBear/seq/core/logger/rotate"
	"github.com/BullionBear/seq/core/model/command"
	"github.com/BullionBear/seq/core/model/event"
)

// MsgLogger writes plaintext JSONL event/command records to a single
// date+size rotated msg_*.jsonl stream. It is designed to be called from the
// single dispatch goroutine, so no locking is required.
type MsgLogger struct {
	w    *rotate.Writer
	buf  []byte
	sync rotate.SyncPolicy
}

// NewMsgLogger creates a MsgLogger backed by pol (ext should be "jsonl").
func NewMsgLogger(pol rotate.Policy) (*MsgLogger, error) {
	if pol.Ext == "" {
		pol.Ext = "jsonl"
	}
	if pol.BaseName == "" {
		pol.BaseName = "msg"
	}
	w, err := rotate.NewWriter(pol)
	if err != nil {
		return nil, fmt.Errorf("msglog: %w", err)
	}
	return &MsgLogger{
		w:    w,
		buf:  make([]byte, 0, 4096),
		sync: pol.Sync,
	}, nil
}

// LogEvent writes a JSONL record for an event.
func (l *MsgLogger) LogEvent(ev Event, payload []byte) {
	b := l.buf[:0]
	b = append(b, `{"kind":"event","topic":"`...)
	b = append(b, ev.Ref.Topic.String()...)
	b = append(b, `","event_id":`...)
	b = strconv.AppendUint(b, ev.EventID, 10)
	b = append(b, `,"created_at":`...)
	b = strconv.AppendUint(b, ev.CreatedAt, 10)
	b = append(b, `,"data":`...)
	b = event.AppendEventJSON(b, ev.Ref.Topic, payload)
	b = append(b, "}\n"...)
	l.buf = b
	if _, err := l.w.Write(b); err != nil {
		log().Error().Err(err).Msg("msglog: failed to write event record")
	}
}

// LogCommand writes a JSONL record for a command.
func (l *MsgLogger) LogCommand(cmd Command, payload []byte) {
	b := l.buf[:0]
	b = append(b, `{"kind":"command","command_type":"`...)
	b = append(b, cmd.Ref.CommandType.String()...)
	b = append(b, `","command_id":`...)
	b = strconv.AppendUint(b, cmd.CommandID, 10)
	b = append(b, `,"created_at":`...)
	b = strconv.AppendUint(b, cmd.CreatedAt, 10)
	b = append(b, `,"data":`...)
	b = command.AppendCommandJSON(b, cmd.Ref.CommandType, payload)
	b = append(b, "}\n"...)
	l.buf = b
	if _, err := l.w.Write(b); err != nil {
		log().Error().Err(err).Msg("msglog: failed to write command record")
	}
}

// Sync fsyncs the active file when SyncPeriodic is configured.
func (l *MsgLogger) Sync() error {
	if l == nil || l.w == nil {
		return nil
	}
	if l.sync != rotate.SyncPeriodic {
		return nil
	}
	return l.w.Sync()
}

// Close syncs and closes the underlying writer.
func (l *MsgLogger) Close() {
	if l == nil || l.w == nil {
		return
	}
	_ = l.w.Close()
	l.w = nil
}
