package msgbus

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"
)

// Msglog binary format (little-endian throughout):
//
// Each .dat file starts with a 64-byte file header:
//
//	magic         [4]byte  "SEQD"
//	endianness    uint16   0xFEFF (a big-endian reader sees 0xFFFE and must refuse the file)
//	schemaVersion uint16   SchemaVersion at write time
//	streamType    uint8    1 = event stream, 2 = command stream
//	reserved      [7]byte  zero
//	build         [48]byte zero-padded VCS revision of the writing binary
//
// followed by records, each with a 24-byte record header:
//
//	schemaVersion uint16  SchemaVersion at write time
//	msgType       uint16  event.Topic or command.CommandType
//	length        uint32  payload length in bytes
//	seqID         uint64  EventID / CommandID
//	createdAt     uint64  UnixNano
//
// SchemaVersion bump rule: any change to the file header, the record header,
// or to the binary encoding of ANY event/command payload type (field added,
// removed, reordered, or resized) MUST increment SchemaVersion. Readers
// refuse files and records whose schema version they do not support.
const (
	// SchemaVersion is the current msglog schema version.
	SchemaVersion uint16 = 1

	// FileHeaderSize is the size of the .dat file header.
	FileHeaderSize = 64

	// RecordHeaderSize is the size of each .dat record header:
	// schemaVersion(2) + msgType(2) + length(4) + seqID(8) + createdAt(8) = 24 bytes
	RecordHeaderSize = 24

	// EndiannessMarker is written as little-endian 0xFEFF.
	EndiannessMarker uint16 = 0xFEFF

	// StreamTypeEvent / StreamTypeCommand identify the record stream in the file header.
	StreamTypeEvent   uint8 = 1
	StreamTypeCommand uint8 = 2
)

// MagicBytes identify a versioned seq .dat file.
var MagicBytes = [4]byte{'S', 'E', 'Q', 'D'}

// File header validation errors.
var (
	ErrBadMagic          = errors.New("msglog: not a seq .dat file (bad magic; unversioned pre-P0-4 files are not supported)")
	ErrBadEndianness     = errors.New("msglog: endianness marker mismatch (file written on a big-endian machine?)")
	ErrUnsupportedSchema = errors.New("msglog: unsupported schema version")
	ErrBadStreamType     = errors.New("msglog: unknown stream type")
)

// FileHeader is the decoded form of the 64-byte .dat file header.
type FileHeader struct {
	SchemaVersion uint16
	StreamType    uint8
	Build         string
}

// EncodeFileHeader renders a file header for the given stream type.
func EncodeFileHeader(streamType uint8) [FileHeaderSize]byte {
	var buf [FileHeaderSize]byte
	copy(buf[0:4], MagicBytes[:])
	binary.LittleEndian.PutUint16(buf[4:6], EndiannessMarker)
	binary.LittleEndian.PutUint16(buf[6:8], SchemaVersion)
	buf[8] = streamType
	// buf[9:16] reserved
	copy(buf[16:64], buildString())
	return buf
}

// DecodeFileHeader parses and validates a file header.
func DecodeFileHeader(buf []byte) (FileHeader, error) {
	if len(buf) < FileHeaderSize {
		return FileHeader{}, fmt.Errorf("msglog: file header truncated (%d bytes)", len(buf))
	}
	if !bytes.Equal(buf[0:4], MagicBytes[:]) {
		return FileHeader{}, ErrBadMagic
	}
	if binary.LittleEndian.Uint16(buf[4:6]) != EndiannessMarker {
		return FileHeader{}, ErrBadEndianness
	}
	h := FileHeader{
		SchemaVersion: binary.LittleEndian.Uint16(buf[6:8]),
		StreamType:    buf[8],
		Build:         string(bytes.TrimRight(buf[16:64], "\x00")),
	}
	if h.SchemaVersion != SchemaVersion {
		return FileHeader{}, fmt.Errorf("%w: file has v%d, reader supports v%d",
			ErrUnsupportedSchema, h.SchemaVersion, SchemaVersion)
	}
	if h.StreamType != StreamTypeEvent && h.StreamType != StreamTypeCommand {
		return FileHeader{}, fmt.Errorf("%w: %d", ErrBadStreamType, h.StreamType)
	}
	return h, nil
}

// buildString returns the VCS revision of the running binary, truncated/padded
// for the fixed-width file header field.
func buildString() []byte {
	rev := "unknown"
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, s := range info.Settings {
			if s.Key == "vcs.revision" {
				rev = s.Value
				break
			}
		}
	}
	if len(rev) > 48 {
		rev = rev[:48]
	}
	return []byte(rev)
}

// MsgLogger writes binary event/command records to date-stamped .dat files.
// It is designed to be called from the single dispatch goroutine, so no
// locking is required.
type MsgLogger struct {
	dir string

	eventFile   *os.File
	eventWriter *bufio.Writer
	eventDate   string

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
	l.writeRecord(l.eventWriter, uint16(ev.Ref.Topic), ev.EventID, ev.CreatedAt, payload)
}

// LogCommand writes a binary record for a command to the command .dat file.
func (l *MsgLogger) LogCommand(cmd Command, payload []byte) {
	today := time.Now().UTC().Format("2006-01-02")
	if l.commandWriter == nil || today != l.commandDate {
		l.rotateCommandFile(today)
	}
	l.writeRecord(l.commandWriter, uint16(cmd.Ref.CommandType), cmd.CommandID, cmd.CreatedAt, payload)
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

// writeRecord writes a single binary record:
// [schemaVersion(2)][msgType(2)][length(4)][seqID(8)][createdAt(8)][payload]
func (l *MsgLogger) writeRecord(w *bufio.Writer, msgType uint16, seqID, createdAt uint64, payload []byte) {
	if w == nil {
		return
	}
	buf := l.headerBuf[:]
	binary.LittleEndian.PutUint16(buf[0:], SchemaVersion)
	binary.LittleEndian.PutUint16(buf[2:], msgType)
	binary.LittleEndian.PutUint32(buf[4:], uint32(len(payload)))
	binary.LittleEndian.PutUint64(buf[8:], seqID)
	binary.LittleEndian.PutUint64(buf[16:], createdAt)
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
	f, err := openLogFile(path, StreamTypeEvent)
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
	f, err := openLogFile(path, StreamTypeCommand)
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

// openLogFile opens (or creates) a .dat file for appending. A new/empty file
// gets a file header. An existing file must carry a compatible header;
// an incompatible or unversioned file is moved aside (never appended to,
// never overwritten) and a fresh file is started.
func openLogFile(path string, streamType uint8) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	if st.Size() == 0 {
		hdr := EncodeFileHeader(streamType)
		if _, err := f.Write(hdr[:]); err != nil {
			f.Close()
			return nil, err
		}
		return f, nil
	}

	var hdr [FileHeaderSize]byte
	_, readErr := f.ReadAt(hdr[:], 0)
	var decoded FileHeader
	var decErr error
	if readErr != nil {
		decErr = readErr
	} else {
		decoded, decErr = DecodeFileHeader(hdr[:])
		if decErr == nil && decoded.StreamType != streamType {
			decErr = fmt.Errorf("%w: file has stream type %d, want %d", ErrBadStreamType, decoded.StreamType, streamType)
		}
	}
	if decErr != nil {
		// Incompatible existing file: preserve it under a suffix and start fresh.
		f.Close()
		backup := fmt.Sprintf("%s.incompatible-%d", path, time.Now().UnixNano())
		log().Warn().Err(decErr).Str("path", path).Str("backup", backup).
			Msg("msglog: existing .dat file has incompatible header; moving aside")
		if err := os.Rename(path, backup); err != nil {
			return nil, fmt.Errorf("msglog: move incompatible file aside: %w", err)
		}
		return openLogFile(path, streamType)
	}
	return f, nil
}
