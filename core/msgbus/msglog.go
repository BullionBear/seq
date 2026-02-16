package msgbus

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	// RecordHeaderSize is the size of each .dat record header:
	// topic/type(4) + sequence_id(8) + created_at(8) + payload_length(4) = 24 bytes
	RecordHeaderSize = 24
)

// MsgLogger writes binary event/command records to date-stamped .dat files.
// It is designed to be called from the single dispatch goroutine, so no
// locking is required.
type MsgLogger struct {
	dir string

	eventFile    *os.File
	eventWriter  *bufio.Writer
	eventDate    string

	commandFile   *os.File
	commandWriter *bufio.Writer
	commandDate   string

	headerBuf [RecordHeaderSize]byte
}

// NewMsgLogger creates a new MsgLogger that writes to the given directory.
// The directory is created if it does not exist.
func NewMsgLogger(dir string) (*MsgLogger, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("msglog: mkdir %s: %w", dir, err)
	}
	return &MsgLogger{dir: dir}, nil
}

// LogEvent writes a binary record for an event to the event .dat file.
func (l *MsgLogger) LogEvent(ev Event, payload []byte) {
	today := time.Now().UTC().Format("2006-01-02")
	if l.eventWriter == nil || today != l.eventDate {
		l.rotateEventFile(today)
	}
	l.writeRecord(l.eventWriter, int32(ev.Ref.Topic), ev.EventID, ev.CreatedAt, payload)
}

// LogCommand writes a binary record for a command to the command .dat file.
func (l *MsgLogger) LogCommand(cmd Command, payload []byte) {
	today := time.Now().UTC().Format("2006-01-02")
	if l.commandWriter == nil || today != l.commandDate {
		l.rotateCommandFile(today)
	}
	l.writeRecord(l.commandWriter, int32(cmd.Ref.CommandType), cmd.CommandID, cmd.CreatedAt, payload)
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

// writeRecord writes a single binary record: [topic(4)][seqID(8)][createdAt(8)][payloadLen(4)][payload]
func (l *MsgLogger) writeRecord(w *bufio.Writer, topic int32, seqID, createdAt uint64, payload []byte) {
	buf := l.headerBuf[:]
	binary.LittleEndian.PutUint32(buf[0:], uint32(topic))
	binary.LittleEndian.PutUint64(buf[4:], seqID)
	binary.LittleEndian.PutUint64(buf[12:], createdAt)
	binary.LittleEndian.PutUint32(buf[20:], uint32(len(payload)))
	w.Write(buf)
	w.Write(payload)
}

func (l *MsgLogger) rotateEventFile(date string) {
	if l.eventWriter != nil {
		l.eventWriter.Flush()
	}
	if l.eventFile != nil {
		l.eventFile.Close()
	}
	path := filepath.Join(l.dir, fmt.Sprintf("event_%s.dat", date))
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log().Error().Err(err).Str("path", path).Msg("msglog: failed to open event file")
		l.eventFile = nil
		l.eventWriter = nil
		return
	}
	l.eventFile = f
	l.eventWriter = bufio.NewWriter(f)
	l.eventDate = date
}

func (l *MsgLogger) rotateCommandFile(date string) {
	if l.commandWriter != nil {
		l.commandWriter.Flush()
	}
	if l.commandFile != nil {
		l.commandFile.Close()
	}
	path := filepath.Join(l.dir, fmt.Sprintf("command_%s.dat", date))
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log().Error().Err(err).Str("path", path).Msg("msglog: failed to open command file")
		l.commandFile = nil
		l.commandWriter = nil
		return
	}
	l.commandFile = f
	l.commandWriter = bufio.NewWriter(f)
	l.commandDate = date
}
