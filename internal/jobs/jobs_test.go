package jobs

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"
)

var idRe = regexp.MustCompile(`id=([0-9a-f]{32})`)

func small(t *testing.T) {
	t.Helper()
	oc, ow, ot, opj, otot := Chunk, Wait, TTL, PerJob, Total
	Chunk, Wait, TTL = 100, 200*time.Millisecond, time.Second
	t.Cleanup(func() { Chunk, Wait, TTL, PerJob, Total = oc, ow, ot, opj, otot })
}

func lines(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteString("line of text\n") // 13 bytes
	}
	return b.String()
}

func TestShortOutputIsWhole(t *testing.T) {
	small(t)
	m := New(t.Context())
	ans, err := m.Start(func(ctx context.Context, w *Writer) (int, error) {
		w.Write([]byte("hello\n"))
		return 0, nil
	})
	if err != nil || ans != "hello\n[exit status 0]" {
		t.Fatalf("got %q, %v", ans, err)
	}
}

func TestPaging(t *testing.T) {
	small(t)
	m := New(t.Context())
	all := lines(30) // 390 bytes = 4 chunks of ≤100
	ans, _ := m.Start(func(ctx context.Context, w *Writer) (int, error) {
		w.Write([]byte(all))
		return 3, nil
	})
	var got strings.Builder
	for i := 0; i < 10; i++ {
		body, _, _ := strings.Cut(ans, "[")
		got.WriteString(body)
		if !strings.Contains(body, "line") || len(body) > Chunk {
			t.Fatalf("chunk %d: %q", i, body)
		}
		if strings.HasSuffix(ans, "[exit status 3]") {
			break
		}
		id := idRe.FindStringSubmatch(ans)
		if id == nil {
			t.Fatalf("no id in %q", ans)
		}
		var err error
		if ans, err = m.More(id[1]); err != nil {
			t.Fatal(err)
		}
	}
	if got.String() != all {
		t.Errorf("reassembled output differs: %d vs %d bytes", got.Len(), len(all))
	}
	if !strings.HasSuffix(ans, "[exit status 3]") {
		t.Errorf("last answer %q", ans)
	}
	// Fully read and finished: the id is gone.
	if len(m.jobs) != 0 {
		t.Errorf("%d jobs kept after the last read", len(m.jobs))
	}
}

func TestSlowCommand(t *testing.T) {
	small(t)
	m := New(t.Context())
	release := make(chan struct{})
	ans, _ := m.Start(func(ctx context.Context, w *Writer) (int, error) {
		w.Write([]byte("first\n"))
		<-release
		w.Write([]byte("second\n"))
		return 0, nil
	})
	if !strings.HasPrefix(ans, "first\n") || !strings.Contains(ans, "still running") {
		t.Fatalf("first answer %q", ans)
	}
	close(release)
	ans, err := m.More(idRe.FindStringSubmatch(ans)[1])
	if err != nil || ans != "second\n[exit status 0]" {
		t.Errorf("second answer %q, %v", ans, err)
	}
}

func TestCapAndExpiry(t *testing.T) {
	small(t)
	PerJob = 50
	m := New(t.Context())
	ans, _ := m.Start(func(ctx context.Context, w *Writer) (int, error) {
		w.Write([]byte(lines(20)))
		return 0, nil
	})
	if !strings.Contains(ans, "output capped") {
		t.Errorf("no cap note: %q", ans)
	}
	ans, _ = m.Start(func(ctx context.Context, w *Writer) (int, error) {
		w.Write([]byte(lines(5)))
		<-ctx.Done()
		return 1, nil
	})
	id := idRe.FindStringSubmatch(ans)[1]
	time.Sleep(TTL + 100*time.Millisecond)
	m.expire()
	if _, err := m.More(id); err != ErrUnknown {
		t.Errorf("expired id: %v", err)
	}
}

func TestBusy(t *testing.T) {
	small(t)
	m := New(t.Context())
	block := make(chan struct{})
	defer close(block)
	for i := 0; i < Running; i++ {
		if _, err := m.Start(func(ctx context.Context, w *Writer) (int, error) { <-block; return 0, nil }); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := m.Start(func(ctx context.Context, w *Writer) (int, error) { return 0, nil }); err != ErrBusy {
		t.Errorf("fifth command: %v, want ErrBusy", err)
	}
}

func TestWallTime(t *testing.T) {
	small(t)
	ow := WallTime
	WallTime = 300 * time.Millisecond
	defer func() { WallTime = ow }()
	m := New(t.Context())
	ans, _ := m.Start(func(ctx context.Context, w *Writer) (int, error) {
		<-ctx.Done()
		return 124, context.Cause(ctx)
	})
	id := idRe.FindStringSubmatch(ans)
	if id != nil {
		ans, _ = m.More(id[1])
	}
	if !strings.Contains(ans, "wall time") || !strings.Contains(ans, "[exit status 124]") {
		t.Errorf("got %q", ans)
	}
}
