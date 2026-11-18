// Package jobs runs commands in the background, keeps their output in RAM
// and hands it out in chunks (the `more` tool).
package jobs

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Limits. Variables so tests can shrink them.
var (
	Chunk    = 30 << 10         // bytes per answer
	Wait     = 15 * time.Second // longest wait per call
	WallTime = 5 * time.Minute  // then the command is killed
	PerJob   = 20 << 20         // bytes kept per command
	Total    = 64 << 20         // bytes kept for all commands
	TTL      = 10 * time.Minute // after the last read
	Running  = 4                // commands at once
)

// ErrBusy means too many commands run already.
var ErrBusy = errors.New("busy: too many commands running, retry shortly")

// ErrUnknown means the id is wrong or expired.
var ErrUnknown = errors.New("unknown or expired id")

// Func is the work of one job. It writes its output to w and returns the
// exit status.
type Func func(ctx context.Context, w *Writer) (int, error)

// Manager holds the jobs of one server.
type Manager struct {
	mu   sync.Mutex
	jobs map[string]*job
	run  chan struct{} // semaphore
	size int           // bytes kept in all jobs
}

type job struct {
	id       string
	started  time.Time
	cancel   context.CancelFunc
	w        *Writer
	cursor   int
	lastRead time.Time
}

// Writer collects a job's output.
type Writer struct {
	mu      sync.Mutex
	buf     bytes.Buffer
	total   int // bytes written, also beyond the cap
	capped  bool
	done    bool
	status  int
	err     error
	changed chan struct{}
	m       *Manager
}

// Write keeps up to PerJob bytes and counts the rest.
func (w *Writer) Write(p []byte) (int, error) {
	w.mu.Lock()
	n := len(p)
	w.total += n
	keep := PerJob - w.buf.Len()
	if keep < n {
		w.capped = true
		if keep < 0 {
			keep = 0
		}
		p = p[:keep]
	}
	w.buf.Write(p)
	w.notify()
	w.mu.Unlock()
	if len(p) > 0 {
		w.m.grow(len(p))
	}
	return n, nil
}

func (w *Writer) notify() {
	close(w.changed)
	w.changed = make(chan struct{})
}

// New returns a manager and starts its janitor; stop it with ctx.
func New(ctx context.Context) *Manager {
	m := &Manager{jobs: map[string]*job{}, run: make(chan struct{}, Running)}
	go func() {
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				m.expire()
			}
		}
	}()
	return m
}

// Start runs f in the background and returns its first answer.
func (m *Manager) Start(f Func) (string, error) {
	select {
	case m.run <- struct{}{}:
	default:
		return "", ErrBusy
	}
	id := newID()
	ctx, cancel := context.WithTimeoutCause(context.Background(), WallTime, fmt.Errorf("wall time of %v reached", WallTime))
	j := &job{id: id, started: time.Now(), cancel: cancel, lastRead: time.Now(),
		w: &Writer{changed: make(chan struct{}), m: m}}
	m.mu.Lock()
	m.jobs[id] = j
	m.mu.Unlock()
	go func() {
		defer func() { <-m.run }()
		defer cancel()
		status, err := f(ctx, j.w)
		j.w.mu.Lock()
		j.w.done, j.w.status, j.w.err = true, status, err
		j.w.notify()
		j.w.mu.Unlock()
	}()
	return m.next(j), nil
}

// More returns the next answer for id.
func (m *Manager) More(id string) (string, error) {
	m.mu.Lock()
	j, ok := m.jobs[id]
	m.mu.Unlock()
	if !ok {
		return "", ErrUnknown
	}
	return m.next(j), nil
}

// next waits until a full chunk is there, the job is done, or Wait passed,
// then returns up to one chunk and a footer.
func (m *Manager) next(j *job) string {
	deadline := time.NewTimer(Wait)
	defer deadline.Stop()
	for {
		j.w.mu.Lock()
		avail := j.w.buf.Len() - j.cursor
		if j.w.done || avail >= Chunk {
			break
		}
		ch := j.w.changed
		j.w.mu.Unlock()
		select {
		case <-ch:
			continue
		case <-deadline.C:
			j.w.mu.Lock()
		}
		break
	}
	finished := false
	defer func() {
		j.w.mu.Unlock()
		if finished {
			m.drop(j) // after the unlock: drop takes m.mu, then j.w.mu
		}
	}()
	data := j.w.buf.Bytes()[j.cursor:]
	if len(data) > Chunk {
		data = data[:Chunk]
		if i := bytes.LastIndexByte(data, '\n'); i > 0 {
			data = data[:i+1] // cut at a line boundary
		}
	}
	j.cursor += len(data)
	j.lastRead = time.Now()
	rest := j.w.buf.Len() - j.cursor
	var b bytes.Buffer
	b.Write(data)
	if len(data) > 0 && data[len(data)-1] != '\n' {
		b.WriteByte('\n')
	}
	switch {
	case j.w.done && rest == 0:
		if j.w.capped {
			fmt.Fprintf(&b, "[output capped: kept %d of %d bytes]\n", j.w.buf.Len(), j.w.total)
		}
		if j.w.err != nil {
			fmt.Fprintf(&b, "[error: %v]\n", j.w.err)
		}
		fmt.Fprintf(&b, "[exit status %d]", j.w.status)
		finished = true
	default:
		state := "finished"
		if !j.w.done {
			state = fmt.Sprintf("still running after %s", time.Since(j.started).Round(time.Second))
		}
		fmt.Fprintf(&b, "[shown %d of %d bytes so far, %s. Call `more` with id=%s for the next part; kept %v after the last read.]",
			j.cursor, j.w.buf.Len(), state, j.id, TTL)
	}
	return b.String()
}

func (m *Manager) grow(n int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.size += n
	for m.size > Total {
		var oldest *job
		for _, j := range m.jobs {
			if oldest == nil || j.started.Before(oldest.started) {
				oldest = j
			}
		}
		if oldest == nil {
			return
		}
		m.dropLocked(oldest)
	}
}

func (m *Manager) drop(j *job) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.dropLocked(j)
}

func (m *Manager) dropLocked(j *job) {
	if _, ok := m.jobs[j.id]; !ok {
		return
	}
	delete(m.jobs, j.id)
	j.cancel()
	j.w.mu.Lock()
	m.size -= j.w.buf.Len()
	j.w.mu.Unlock()
}

func (m *Manager) expire() {
	m.mu.Lock()
	var old []*job
	for _, j := range m.jobs {
		if time.Since(j.lastRead) > TTL {
			old = append(old, j)
		}
	}
	m.mu.Unlock()
	for _, j := range old {
		m.drop(j)
	}
}

func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
