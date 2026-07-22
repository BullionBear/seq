package rotate

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// Writer is an io.Writer that rotates on UTC date and/or size.
// Steady-state Write is lock-free (atomic active pointer + size); the mutex
// is taken only on the rare rotation path. Writes go straight to *os.File
// with O_APPEND (no userspace buffering) so a process crash cannot lose a
// completed write that already returned.
type Writer struct {
	pol Policy

	active atomic.Pointer[os.File]
	size   atomic.Int64
	curDay atomic.Int64 // UTC epoch-day of the active file; read on hot path

	mu       sync.Mutex
	curSeq   int
	retiring []*os.File
	closed   bool

	// nowDay is overridable in tests; nil uses time.Now().Unix()/86400.
	nowDay func() int64
}

// NewWriter creates a Writer and opens (or appends to) today's active file.
func NewWriter(pol Policy) (*Writer, error) {
	if pol.Dir == "" {
		return nil, fmt.Errorf("rotate: empty Dir")
	}
	if pol.BaseName == "" {
		return nil, fmt.Errorf("rotate: empty BaseName")
	}
	if pol.Ext == "" {
		return nil, fmt.Errorf("rotate: empty Ext")
	}
	if pol.Sync == "" {
		pol.Sync = SyncRotate
	}
	if err := os.MkdirAll(pol.Dir, 0755); err != nil {
		return nil, fmt.Errorf("rotate: mkdir %s: %w", pol.Dir, err)
	}

	rw := &Writer{pol: pol}
	day := rw.day()
	f, seq, size, err := openLatest(pol, day)
	if err != nil {
		return nil, err
	}
	rw.active.Store(f)
	rw.size.Store(size)
	rw.curDay.Store(day)
	rw.curSeq = seq
	return rw, nil
}

// Write appends p to the active file, rotating first if needed.
func (rw *Writer) Write(p []byte) (int, error) {
	day := rw.day()
	sz := rw.size.Load()
	if rw.needRotate(day, sz, len(p)) {
		if err := rw.rotate(day); err != nil {
			return 0, err
		}
	}

	f := rw.active.Load()
	if f == nil {
		return 0, fmt.Errorf("rotate: writer closed")
	}
	n, err := f.Write(p)
	if n > 0 {
		rw.size.Add(int64(n))
	}
	if err == nil && rw.pol.Sync == SyncEach {
		err = f.Sync()
	}
	return n, err
}

// Sync fsyncs the active file. Used by SyncPeriodic callers.
func (rw *Writer) Sync() error {
	f := rw.active.Load()
	if f == nil {
		return nil
	}
	return f.Sync()
}

// Close syncs and closes the active file and any retiring fds.
func (rw *Writer) Close() error {
	rw.mu.Lock()
	defer rw.mu.Unlock()
	if rw.closed {
		return nil
	}
	rw.closed = true

	var firstErr error
	for _, old := range rw.retiring {
		if err := old.Sync(); err != nil && firstErr == nil {
			firstErr = err
		}
		if err := old.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	rw.retiring = nil

	f := rw.active.Swap(nil)
	if f != nil {
		if rw.pol.Sync != SyncNone {
			if err := f.Sync(); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		if err := f.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// ActiveName returns the basename of the current active file (for tests/cleanup).
func (rw *Writer) ActiveName() string {
	f := rw.active.Load()
	if f == nil {
		return ""
	}
	return filepath.Base(f.Name())
}

// Policy returns a copy of the writer policy.
func (rw *Writer) Policy() Policy {
	return rw.pol
}

func (rw *Writer) day() int64 {
	if rw.nowDay != nil {
		return rw.nowDay()
	}
	return time.Now().Unix() / 86400
}

func (rw *Writer) needRotate(day int64, sz int64, upcoming int) bool {
	if rw.pol.Daily && day != rw.curDay.Load() {
		return true
	}
	if rw.pol.MaxBytes > 0 && sz+int64(upcoming) > rw.pol.MaxBytes {
		return true
	}
	return false
}

func (rw *Writer) rotate(day int64) error {
	rw.mu.Lock()
	defer rw.mu.Unlock()

	if rw.closed {
		return fmt.Errorf("rotate: writer closed")
	}
	if !rw.needRotate(day, rw.size.Load(), 0) {
		return nil
	}

	// Close the previous generation (epoch-delayed close).
	for _, old := range rw.retiring {
		if rw.pol.Sync == SyncRotate || rw.pol.Sync == SyncEach {
			_ = old.Sync()
		}
		_ = old.Close()
	}
	rw.retiring = rw.retiring[:0]

	prev := rw.active.Load()
	prevName := ""
	if prev != nil {
		prevName = filepath.Base(prev.Name())
	}

	newDay := day
	newSeq := 0
	if day == rw.curDay.Load() {
		// Same-day size rotation: bump sequence.
		newSeq = rw.curSeq + 1
	}

	newf, err := openSeq(rw.pol, newDay, newSeq)
	if err != nil {
		return err
	}
	rw.active.Store(newf)
	rw.size.Store(0)
	rw.curDay.Store(newDay)
	rw.curSeq = newSeq

	if prev != nil {
		rw.retiring = append(rw.retiring, prev)
	}

	go cleanup(rw.pol, filepath.Base(newf.Name()))
	_ = prevName
	return nil
}

// openLatest finds the highest-seq file for day and opens it for append,
// or creates the base file if none exist.
func openLatest(pol Policy, day int64) (*os.File, int, int64, error) {
	date := dayString(day)
	highest := -1
	var highestPath string

	entries, err := os.ReadDir(pol.Dir)
	if err != nil && !os.IsNotExist(err) {
		return nil, 0, 0, err
	}
	prefix := pol.BaseName + "_" + date
	suffix := "." + pol.Ext
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !stringsHasPrefixSuffix(name, prefix, suffix) {
			continue
		}
		seq, ok := parseSeq(name, prefix, suffix)
		if !ok {
			continue
		}
		if seq > highest {
			highest = seq
			highestPath = filepath.Join(pol.Dir, name)
		}
	}

	if highest < 0 {
		f, err := openSeq(pol, day, 0)
		if err != nil {
			return nil, 0, 0, err
		}
		return f, 0, 0, nil
	}

	f, err := os.OpenFile(highestPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, 0, 0, err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, 0, 0, err
	}
	return f, highest, info.Size(), nil
}

func openSeq(pol Policy, day int64, seq int) (*os.File, error) {
	path := filepath.Join(pol.Dir, fileName(pol.BaseName, dayString(day), seq, pol.Ext))
	return os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
}

func fileName(base, date string, seq int, ext string) string {
	if seq <= 0 {
		return fmt.Sprintf("%s_%s.%s", base, date, ext)
	}
	return fmt.Sprintf("%s_%s.%d.%s", base, date, seq, ext)
}

func dayString(day int64) string {
	t := time.Unix(day*86400, 0).UTC()
	return t.Format("2006-01-02")
}

func stringsHasPrefixSuffix(s, prefix, suffix string) bool {
	return len(s) >= len(prefix)+len(suffix) &&
		s[:len(prefix)] == prefix &&
		s[len(s)-len(suffix):] == suffix
}

// parseSeq extracts the sequence number from names like:
//
//	base_DATE.ext       → 0
//	base_DATE.N.ext     → N
func parseSeq(name, prefix, suffix string) (int, bool) {
	mid := name[len(prefix) : len(name)-len(suffix)]
	if mid == "" {
		// "base_DATE.ext" — prefix is "base_DATE", mid empty before ".ext"... 
		// Actually name = prefix + mid + suffix where prefix = "base_DATE".
		// For base_DATE.ext: mid is empty → seq 0.
		return 0, true
	}
	if mid[0] != '.' {
		return 0, false
	}
	mid = mid[1:] // strip leading '.'
	if mid == "" {
		return 0, false
	}
	n := 0
	for i := 0; i < len(mid); i++ {
		c := mid[i]
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
	}
	return n, true
}
