package rotate

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func testPolicy(dir string) Policy {
	return Policy{
		Dir:      dir,
		BaseName: "seq",
		Ext:      "log",
		MaxBytes: 0,
		Daily:    true,
		Sync:     SyncNone,
	}
}

func TestWriter_DailyRotate(t *testing.T) {
	dir := t.TempDir()
	pol := testPolicy(dir)
	w, err := NewWriter(pol)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	day := w.curDay.Load()
	w.nowDay = func() int64 { return day }
	if _, err := w.Write([]byte("line-a\n")); err != nil {
		t.Fatal(err)
	}
	name0 := w.ActiveName()

	w.nowDay = func() int64 { return day + 1 }
	if _, err := w.Write([]byte("line-b\n")); err != nil {
		t.Fatal(err)
	}
	name1 := w.ActiveName()
	if name0 == name1 {
		t.Fatalf("expected date rotation, still %s", name1)
	}
	if !strings.Contains(name1, dayString(day+1)) {
		t.Fatalf("active=%s, want date %s", name1, dayString(day+1))
	}
}

func TestWriter_SizeRotate(t *testing.T) {
	dir := t.TempDir()
	pol := testPolicy(dir)
	pol.Daily = false
	pol.MaxBytes = 32
	w, err := NewWriter(pol)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	first := w.ActiveName()
	payload := append(bytes.Repeat([]byte("x"), 40), '\n')
	if _, err := w.Write(payload); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("y\n")); err != nil {
		t.Fatal(err)
	}
	second := w.ActiveName()
	if first == second {
		t.Fatalf("expected size rotation, still %s", first)
	}
	if !strings.Contains(second, ".1.") {
		t.Fatalf("expected seq file, got %s", second)
	}
}

func TestWriter_DateAndSize(t *testing.T) {
	dir := t.TempDir()
	pol := testPolicy(dir)
	pol.MaxBytes = 64
	pol.Daily = true
	w, err := NewWriter(pol)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	day := w.curDay.Load()
	w.nowDay = func() int64 { return day }

	big := append(bytes.Repeat([]byte("a"), 80), '\n')
	if _, err := w.Write(big); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("b\n")); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(w.ActiveName(), ".1.") {
		t.Fatalf("want size seq, got %s", w.ActiveName())
	}

	w.nowDay = func() int64 { return day + 1 }
	if _, err := w.Write([]byte("c\n")); err != nil {
		t.Fatal(err)
	}
	active := w.ActiveName()
	if strings.Contains(active, ".1.") {
		t.Fatalf("new day should reset seq, got %s", active)
	}
	if !strings.Contains(active, dayString(day+1)) {
		t.Fatalf("active=%s want day %s", active, dayString(day+1))
	}
}

func TestWriter_ConcurrentNoInterleave(t *testing.T) {
	dir := t.TempDir()
	pol := testPolicy(dir)
	pol.Daily = false
	w, err := NewWriter(pol)
	if err != nil {
		t.Fatal(err)
	}

	const goroutines = 8
	const perG = 200
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			line := fmt.Sprintf("G%d-XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX\n", id)
			for i := 0; i < perG; i++ {
				if _, err := w.Write([]byte(line)); err != nil {
					t.Errorf("write: %v", err)
					return
				}
			}
		}(g)
	}
	wg.Wait()
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var all []byte
	for _, e := range entries {
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		all = append(all, b...)
	}
	sc := bufio.NewScanner(bytes.NewReader(all))
	n := 0
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "G") || !strings.Contains(line, "-XXXX") {
			t.Fatalf("interleaved/corrupt line: %q", line)
		}
		n++
	}
	if n != goroutines*perG {
		t.Fatalf("lines=%d want %d", n, goroutines*perG)
	}
}

func TestWriter_SteadyStateZeroAlloc(t *testing.T) {
	dir := t.TempDir()
	pol := testPolicy(dir)
	pol.Daily = false
	pol.MaxBytes = 0
	w, err := NewWriter(pol)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	msg := []byte("hello steady path\n")
	_, _ = w.Write(msg)
	allocs := testing.AllocsPerRun(1000, func() {
		_, _ = w.Write(msg)
	})
	if allocs != 0 {
		t.Fatalf("allocs/op = %v, want 0", allocs)
	}
}

func BenchmarkRotateWriter_Write(b *testing.B) {
	dir := b.TempDir()
	pol := testPolicy(dir)
	pol.Daily = false
	w, err := NewWriter(pol)
	if err != nil {
		b.Fatal(err)
	}
	defer w.Close()
	msg := []byte("benchmark-line-0123456789\n")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := w.Write(msg); err != nil {
			b.Fatal(err)
		}
	}
}

func TestWriter_CrashDurability(t *testing.T) {
	if os.Getenv("ROTATE_CRASH_CHILD") == "1" {
		dir := os.Getenv("ROTATE_CRASH_DIR")
		w, err := NewWriter(Policy{
			Dir: dir, BaseName: "crash", Ext: "log",
			Daily: false, Sync: SyncNone,
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		for i := 0; i < 5000; i++ {
			line := fmt.Sprintf("crash-line-%05d-ABCDEFGHIJKLMNOPQRSTUVWXYZ\n", i)
			if _, err := w.Write([]byte(line)); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(2)
			}
		}
		fmt.Println("READY")
		select {}
	}

	dir := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestWriter_CrashDurability$")
	cmd.Env = append(os.Environ(),
		"ROTATE_CRASH_CHILD=1",
		"ROTATE_CRASH_DIR="+dir,
	)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	ready := make(chan error, 1)
	go func() {
		buf := make([]byte, 64)
		n, err := stdout.Read(buf)
		if err != nil {
			ready <- err
			return
		}
		if !strings.Contains(string(buf[:n]), "READY") {
			ready <- fmt.Errorf("unexpected stdout: %q", buf[:n])
			return
		}
		ready <- nil
	}()

	select {
	case err := <-ready:
		if err != nil {
			_ = cmd.Process.Kill()
			t.Fatalf("child: %v", err)
		}
	case <-time.After(10 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("child did not become ready")
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("no crash log written")
	}
	data, err := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 || data[len(data)-1] != '\n' {
		t.Fatalf("truncated tail: len=%d", len(data))
	}
	sc := bufio.NewScanner(bytes.NewReader(data))
	n := 0
	for sc.Scan() {
		if !strings.HasPrefix(sc.Text(), "crash-line-") {
			t.Fatalf("corrupt line: %q", sc.Text())
		}
		n++
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Fatal("no complete lines after SIGKILL")
	}
}

func TestWriter_RetentionOnRotate(t *testing.T) {
	dir := t.TempDir()
	pol := testPolicy(dir)
	pol.Daily = false
	pol.MaxBytes = 16
	pol.MaxBackups = 1
	w, err := NewWriter(pol)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	for i := 0; i < 6; i++ {
		payload := append(bytes.Repeat([]byte("z"), 20), '\n')
		if _, err := w.Write(payload); err != nil {
			t.Fatal(err)
		}
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		entries, _ := os.ReadDir(dir)
		if len(entries) <= 2 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	entries, _ := os.ReadDir(dir)
	t.Fatalf("retention did not shrink enough: %d files", len(entries))
}
