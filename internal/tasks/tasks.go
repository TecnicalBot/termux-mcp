// Package tasks implements Module I: long-running background tasks that
// outlive a single MCP tool call.
//
// A task is spawned with sh -c, detached into its own session/process group
// (setsid) so it keeps running even if the MCP client disconnects or the
// server restarts. Its stdout/stderr are redirected into capped log files and
// state is persisted as JSON, so tasks survive server restarts and can be
// polled with Status and read with Log.
package tasks

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Task states.
const (
	StateRunning     = "running"
	StateFinished    = "finished"
	StateFailed      = "failed"
	StateStopped     = "stopped"     // stopped via stop_task
	StateInterrupted = "interrupted" // process died without a clean exit (e.g. server restarted)
)

// ErrNotFound is returned for unknown task IDs.
var ErrNotFound = errors.New("task not found")

const truncMarker = "\n...[task log truncated]\n"

// TaskInfo is the public, JSON-serializable view of a task.
type TaskInfo struct {
	ID         string    `json:"id"`
	Command    string    `json:"command"`
	State      string    `json:"state"`
	PID        int       `json:"pid"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at,omitempty"`
	ExitCode   int       `json:"exit_code,omitempty"`
	Error      string    `json:"error,omitempty"`
	StdoutLog  string    `json:"stdout_log"`
	StderrLog  string    `json:"stderr_log"`
	DurationMS int64     `json:"duration_ms,omitempty"`
}

// Manager tracks and controls background tasks. It is safe for concurrent use.
type Manager struct {
	dir           string // base dir; each task lives in <dir>/<id>/
	maxConcurrent int
	maxLogBytes   int64 // cap per stream on disk

	mu   sync.Mutex
	live map[string]*task // tasks spawned by this process
	seq  int64
}

// task is the in-memory handle for a running task.
type task struct {
	info   TaskInfo
	cmd    *exec.Cmd
	killed bool // stop_task requested
}

// New creates a Manager persisting state under dir (created lazily).
// maxConcurrent bounds simultaneously running tasks (<=0 means 8);
// maxLogBytes caps each log stream (<=0 means 8 MiB).
func New(dir string, maxConcurrent int, maxLogBytes int64) *Manager {
	if maxConcurrent <= 0 {
		maxConcurrent = 8
	}
	if maxLogBytes <= 0 {
		maxLogBytes = 8 << 20
	}
	if dir == "" {
		dir = os.TempDir()
	}
	m := &Manager{dir: dir, maxConcurrent: maxConcurrent, maxLogBytes: maxLogBytes, live: make(map[string]*task)}
	// Task directories deliberately outlive this process (see config.defaultDataDir),
	// but the ID counter does not, so it has to be seeded from what is on disk.
	// Reusing an ID would make the new task append to the previous one's logs,
	// and task_log would then report the old run's output as this run's.
	m.seq = highestExistingID(dir)
	return m
}

// highestExistingID returns the largest task number already persisted in dir,
// or 0 when the directory is empty, absent or unreadable.
func highestExistingID(dir string) int64 {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	var max int64
	for _, e := range entries {
		if n, ok := parseTaskID(e.Name()); ok && n > max {
			max = n
		}
	}
	return max
}

// parseTaskID decodes the numeric suffix of a "tNNNNNN" task directory name.
func parseTaskID(name string) (int64, bool) {
	if len(name) < 2 || name[0] != 't' {
		return 0, false
	}
	n, err := strconv.ParseInt(name[1:], 10, 64)
	return n, err == nil
}

// Dir returns the base directory where task state is persisted.
func (m *Manager) Dir() string { return m.dir }

// Start launches command via sh -c as a detached background task and returns
// its initial info. The task is NOT tied to the caller's context or the MCP
// session: it keeps running after the tool call returns.
func (m *Manager) Start(command, workdir string) (TaskInfo, error) {
	if strings.TrimSpace(command) == "" {
		return TaskInfo{}, errors.New("empty command")
	}

	m.mu.Lock()
	if len(m.live) >= m.maxConcurrent {
		m.mu.Unlock()
		return TaskInfo{}, fmt.Errorf("task limit reached (%d running); stop or delete a task first", m.maxConcurrent)
	}
	id, tdir, err := m.allocateLocked()
	if err != nil {
		m.mu.Unlock()
		return TaskInfo{}, err
	}
	t := &task{info: TaskInfo{
		ID:        id,
		Command:   command,
		State:     StateRunning,
		StartedAt: time.Now(),
		StdoutLog: filepath.Join(tdir, "stdout.log"),
		StderrLog: filepath.Join(tdir, "stderr.log"),
	}}
	m.live[id] = t
	m.mu.Unlock()

	outF, err := os.OpenFile(filepath.Join(tdir, "stdout.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		m.failStart(id, tdir)
		return TaskInfo{}, err
	}
	errF, err := os.OpenFile(filepath.Join(tdir, "stderr.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		_ = outF.Close()
		m.failStart(id, tdir)
		return TaskInfo{}, err
	}

	cmd := exec.Command("sh", "-c", command)
	cmd.Env = os.Environ()
	if workdir != "" {
		cmd.Dir = workdir
	}
	// Detach into its own process group so the task survives us and can be
	// signaled as a unit (SIGTERM/SIGKILL to -pid).
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdout = &cappedWriter{f: outF, remain: m.maxLogBytes}
	cmd.Stderr = &cappedWriter{f: errF, remain: m.maxLogBytes}

	if err := cmd.Start(); err != nil {
		_ = outF.Close()
		_ = errF.Close()
		m.failStart(id, tdir)
		return TaskInfo{}, fmt.Errorf("spawn task: %w", err)
	}

	t.cmd = cmd
	t.info.PID = cmd.Process.Pid
	_ = m.writeMeta(t)
	go m.reap(t, outF, errF)
	return t.info, nil
}

// failStart removes a half-created task after a spawn failure.
func (m *Manager) failStart(id, tdir string) {
	m.mu.Lock()
	delete(m.live, id)
	m.mu.Unlock()
	_ = os.RemoveAll(tdir)
}

// maxIDAttempts bounds the search for a free task ID. Seeding the counter from
// disk means the first candidate is normally free, so this only matters when
// another server process is writing into the same directory.
const maxIDAttempts = 1000

// allocateLocked reserves the next unused task ID and creates its directory.
// Callers must hold m.mu.
//
// The directory is created here rather than after unlocking so that claiming an
// ID is a single atomic mkdir. Anything else leaves a window in which a restart
// can hand out an ID whose logs still hold an earlier run's output, and
// task_log has no way to tell the two apart.
func (m *Manager) allocateLocked() (string, string, error) {
	if err := os.MkdirAll(m.dir, 0o755); err != nil {
		return "", "", fmt.Errorf("create task dir: %w", err)
	}

	for attempt := 0; attempt < maxIDAttempts; attempt++ {
		m.seq++
		id := fmt.Sprintf("t%06d", m.seq)
		tdir := filepath.Join(m.dir, id)

		err := os.Mkdir(tdir, 0o755)
		if err == nil {
			return id, tdir, nil
		}
		if !os.IsExist(err) {
			return "", "", fmt.Errorf("create task dir: %w", err)
		}
		// Already claimed by a task New's scan did not see. Try the next ID
		// rather than reusing it: its logs belong to somebody else.
	}

	return "", "", fmt.Errorf("no free task id under %s after %d attempts", m.dir, maxIDAttempts)
}

// reap waits for the process, finalizes state, and persists it.
func (m *Manager) reap(t *task, outF, errF *os.File) {
	err := t.cmd.Wait()
	_ = outF.Close()
	_ = errF.Close()

	m.mu.Lock()
	killed := t.killed
	delete(m.live, t.info.ID)
	t.info.FinishedAt = time.Now()
	t.info.DurationMS = t.info.FinishedAt.Sub(t.info.StartedAt).Milliseconds()

	code := 0
	msg := ""
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			code = ee.ExitCode()
			if code < 0 {
				msg = err.Error()
			}
		} else {
			msg = err.Error()
		}
	}
	switch {
	case killed:
		t.info.State = StateStopped
	case code == 0 && msg == "":
		t.info.State = StateFinished
	default:
		t.info.State = StateFailed
	}
	t.info.ExitCode = code
	t.info.Error = msg
	info := t.info
	m.mu.Unlock()

	_ = m.writeMetaInfo(info)
}

// Status returns the current info for a task, repairing stale "running"
// entries left behind by a server restart (dead process => interrupted).
func (m *Manager) Status(id string) (TaskInfo, error) {
	m.mu.Lock()
	if t, ok := m.live[id]; ok {
		info := t.info
		m.mu.Unlock()
		return info, nil
	}
	m.mu.Unlock()
	return m.readMeta(id)
}

// List returns all known tasks, live and persisted, newest first.
func (m *Manager) List() ([]TaskInfo, error) {
	m.mu.Lock()
	out := make([]TaskInfo, 0, len(m.live))
	seen := make(map[string]bool, len(m.live))
	for _, t := range m.live {
		out = append(out, t.info)
		seen[t.info.ID] = true
	}
	m.mu.Unlock()

	entries, err := os.ReadDir(m.dir)
	if err != nil {
		if os.IsNotExist(err) {
			sortTasks(out)
			return out, nil
		}
		return nil, err
	}
	for _, e := range entries {
		if !e.IsDir() || seen[e.Name()] {
			continue
		}
		if info, err := m.readMeta(e.Name()); err == nil {
			out = append(out, info)
		}
	}
	sortTasks(out)
	return out, nil
}

func sortTasks(ts []TaskInfo) {
	sort.Slice(ts, func(i, j int) bool { return ts[i].StartedAt.After(ts[j].StartedAt) })
}

// Log returns the requested output stream(s) of a task. stream is one of
// "stdout", "stderr" or "all" (default). tailBytes bounds each stream; <=0
// means a sensible default tail (64 KiB). The whole file is returned when it
// fits within the limit.
func (m *Manager) Log(id, stream string, tailBytes int64) (string, error) {
	if _, err := m.Status(id); err != nil {
		return "", err
	}
	switch stream {
	case "", "all", "stdout", "stderr":
	default:
		return "", fmt.Errorf("stream must be one of \"stdout\", \"stderr\", \"all\"; got %q", stream)
	}
	if tailBytes <= 0 {
		tailBytes = 64 << 10
	}
	if tailBytes > m.maxLogBytes {
		tailBytes = m.maxLogBytes
	}

	dir := filepath.Join(m.dir, id)
	out, otrunc, err := tailFile(filepath.Join(dir, "stdout.log"), tailBytes)
	if err != nil {
		return "", err
	}
	errT, etrunc, err := tailFile(filepath.Join(dir, "stderr.log"), tailBytes)
	if err != nil {
		return "", err
	}

	switch stream {
	case "stdout":
		return withTruncNote(out, otrunc), nil
	case "stderr":
		return withTruncNote(errT, etrunc), nil
	}
	// all
	if out == "" {
		return withTruncNote(errT, etrunc), nil
	}
	if errT == "" {
		return withTruncNote(out, otrunc), nil
	}
	return withTruncNote(out, otrunc) + "\n\n--- stderr ---\n" + withTruncNote(errT, etrunc), nil
}

func withTruncNote(s string, truncated bool) string {
	if truncated {
		return "[...earlier output omitted...]\n" + s
	}
	return s
}

// tailFile reads the last n bytes of a file, starting at a line boundary.
func tailFile(path string, n int64) (string, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, err
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return "", false, err
	}
	size := st.Size()
	if size == 0 {
		return "", false, nil
	}
	if size <= n {
		b, err := io.ReadAll(f)
		return string(b), false, err
	}
	if _, err := f.Seek(size-n, io.SeekStart); err != nil {
		return "", false, err
	}
	// Skip the partial first line so we never cut mid-line.
	head := make([]byte, 4096)
	if nr, err := f.Read(head); err == nil || nr > 0 {
		if i := bytes.IndexByte(head[:nr], '\n'); i >= 0 {
			if _, err := f.Seek(size-n+int64(i+1), io.SeekStart); err != nil {
				return "", false, err
			}
		} else {
			// No newline in the first chunk: just start from the beginning.
			if _, err := f.Seek(size-n, io.SeekStart); err != nil {
				return "", false, err
			}
		}
	}
	b, err := io.ReadAll(f)
	return string(b), true, err
}

// Stop requests a task to terminate: SIGTERM to its whole process group,
// escalated to SIGKILL after a short grace period. It returns the task info
// after waiting briefly for the process to exit.
func (m *Manager) Stop(id string) (TaskInfo, error) {
	m.mu.Lock()
	t, ok := m.live[id]
	if !ok {
		m.mu.Unlock()
		info, err := m.readMeta(id)
		if err != nil {
			return TaskInfo{}, err
		}
		if info.State == StateRunning {
			return info, fmt.Errorf("task %s is running but not managed by this server instance", id)
		}
		return info, fmt.Errorf("task %s is not running (state %s)", id, info.State)
	}
	t.killed = true
	pid := t.cmd.Process.Pid
	m.mu.Unlock()

	signalGroup(pid, syscall.SIGTERM)

	// Escalate to SIGKILL if the process group does not exit on its own.
	go func() {
		time.Sleep(5 * time.Second)
		m.mu.Lock()
		_, still := m.live[id]
		m.mu.Unlock()
		if still {
			signalGroup(pid, syscall.SIGKILL)
		}
	}()

	// Wait briefly for the process to actually exit so the returned info is
	// usually already in its final state.
	deadline := time.Now().Add(1500 * time.Millisecond)
	for time.Now().Before(deadline) {
		info, err := m.Status(id)
		if err == nil && info.State != StateRunning {
			return info, nil
		}
		time.Sleep(25 * time.Millisecond)
	}
	return m.Status(id)
}

// signalGroup sends sig to the process group led by pid (created via setsid).
func signalGroup(pid int, sig syscall.Signal) {
	if err := syscall.Kill(-pid, sig); err != nil && err != syscall.ESRCH {
		_ = syscall.Kill(pid, sig)
	}
}

// Delete removes a finished task and its logs. Running tasks are refused.
func (m *Manager) Delete(id string) error {
	m.mu.Lock()
	if _, ok := m.live[id]; ok {
		m.mu.Unlock()
		return fmt.Errorf("task %s is still running; stop it first", id)
	}
	m.mu.Unlock()

	info, err := m.readMeta(id)
	if err != nil {
		return err
	}
	if info.State == StateRunning {
		return fmt.Errorf("task %s is still running; stop it first", id)
	}
	return os.RemoveAll(filepath.Join(m.dir, id))
}

// readMeta reads a persisted task, repairing stale "running" entries whose
// process is no longer alive (e.g. after a server restart).
func (m *Manager) readMeta(id string) (TaskInfo, error) {
	dir := filepath.Join(m.dir, id)
	b, err := os.ReadFile(filepath.Join(dir, "meta.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return TaskInfo{}, ErrNotFound
		}
		return TaskInfo{}, err
	}
	var info TaskInfo
	if err := json.Unmarshal(b, &info); err != nil {
		return TaskInfo{}, fmt.Errorf("task %s: corrupt meta: %w", id, err)
	}
	if info.State == StateRunning && info.PID > 0 && !processAlive(info.PID) {
		info.State = StateInterrupted
		info.Error = "process is no longer alive (server restarted?)"
		info.FinishedAt = time.Now()
		info.DurationMS = info.FinishedAt.Sub(info.StartedAt).Milliseconds()
		_ = m.writeMetaInfo(info)
	}
	return info, nil
}

// processAlive reports whether pid exists (kill 0 probe). PID reuse can make
// this report a false positive; that is accepted for liveness heuristics.
func processAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

func (m *Manager) writeMeta(t *task) error {
	return m.writeMetaInfo(t.info)
}

func (m *Manager) writeMetaInfo(info TaskInfo) error {
	b, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(m.dir, info.ID, "meta.json"), b, 0o644)
}

// cappedWriter writes to f until maxBytes have been written, then appends a
// marker once and drops further output, so runaway tasks cannot fill the disk.
type cappedWriter struct {
	f      *os.File
	remain int64
	marked bool
}

func (c *cappedWriter) Write(p []byte) (int, error) {
	n := len(p)
	if c.remain > 0 {
		w := p
		if int64(len(w)) > c.remain {
			w = w[:c.remain]
			c.remain = 0
		} else {
			c.remain -= int64(len(w))
		}
		if _, err := c.f.Write(w); err != nil {
			return 0, err
		}
	}
	if c.remain <= 0 && !c.marked {
		c.marked = true
		_, _ = c.f.WriteString(truncMarker)
	}
	return n, nil
}
