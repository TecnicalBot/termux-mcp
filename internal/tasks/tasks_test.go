package tasks

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// waitState polls Status until the task reaches the wanted state or times out.
func waitState(t *testing.T, m *Manager, id, want string, timeout time.Duration) TaskInfo {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		info, err := m.Status(id)
		if err != nil {
			t.Fatalf("status(%s): %v", id, err)
		}
		if info.State == want {
			return info
		}
		if time.Now().After(deadline) {
			t.Fatalf("task %s: state %s, want %s (last info: %+v)", id, info.State, want, info)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestStartFinishLogDelete(t *testing.T) {
	m := New(t.TempDir(), 4, 1<<20)
	info, err := m.Start("echo hello task", "")
	if err != nil {
		t.Fatal(err)
	}
	if info.State != StateRunning {
		t.Fatalf("initial state = %s, want running", info.State)
	}
	if info.PID <= 0 {
		t.Fatalf("pid = %d, want > 0", info.PID)
	}
	final := waitState(t, m, info.ID, StateFinished, 5*time.Second)
	if final.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", final.ExitCode)
	}
	if final.DurationMS < 0 {
		t.Fatalf("duration_ms = %d, want >= 0", final.DurationMS)
	}
	log, err := m.Log(info.ID, "all", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(log, "hello task") {
		t.Fatalf("log = %q, want it to contain task output", log)
	}
	if err := m.Delete(info.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Status(info.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("status after delete: err = %v, want ErrNotFound", err)
	}
}

func TestFailedExitCode(t *testing.T) {
	m := New(t.TempDir(), 4, 1<<20)
	info, err := m.Start("exit 7", "")
	if err != nil {
		t.Fatal(err)
	}
	final := waitState(t, m, info.ID, StateFailed, 5*time.Second)
	if final.ExitCode != 7 {
		t.Fatalf("exit code = %d, want 7", final.ExitCode)
	}
}

func TestLogStreamsAndTail(t *testing.T) {
	m := New(t.TempDir(), 4, 1<<20)
	info, err := m.Start("echo out-line; echo err-line >&2", "")
	if err != nil {
		t.Fatal(err)
	}
	waitState(t, m, info.ID, StateFinished, 5*time.Second)

	all, err := m.Log(info.ID, "all", 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"out-line", "err-line", "--- stderr ---"} {
		if !strings.Contains(all, want) {
			t.Fatalf("all log missing %q:\n%s", want, all)
		}
	}

	onlyErr, err := m.Log(info.ID, "stderr", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(onlyErr, "err-line") || strings.Contains(onlyErr, "out-line") {
		t.Fatalf("stderr log wrong:\n%s", onlyErr)
	}

	tail, err := m.Log(info.ID, "stdout", 4)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(tail, "[...earlier output omitted...]") {
		t.Fatalf("tail log missing truncation note:\n%s", tail)
	}
}

func TestStop(t *testing.T) {
	m := New(t.TempDir(), 4, 1<<20)
	info, err := m.Start("sleep 60", "")
	if err != nil {
		t.Fatal(err)
	}
	stopped, err := m.Stop(info.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stopped.State != StateStopped {
		t.Fatalf("state after stop = %s, want stopped (info: %+v)", stopped.State, stopped)
	}
	if err := m.Delete(info.ID); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteRunningRefused(t *testing.T) {
	m := New(t.TempDir(), 4, 1<<20)
	info, err := m.Start("sleep 60", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Delete(info.ID); err == nil {
		t.Fatal("Delete of a running task must fail")
	}
	_, _ = m.Stop(info.ID)
}

func TestConcurrencyLimit(t *testing.T) {
	m := New(t.TempDir(), 2, 1<<20)
	ids := make([]string, 0, 2)
	for i := 0; i < 2; i++ {
		info, err := m.Start("sleep 60", "")
		if err != nil {
			t.Fatalf("start %d: %v", i, err)
		}
		ids = append(ids, info.ID)
	}
	if _, err := m.Start("echo x", ""); err == nil {
		t.Fatal("want task-limit error on third start")
	}
	for _, id := range ids {
		_, _ = m.Stop(id)
	}
}

func TestEmptyCommandRejected(t *testing.T) {
	m := New(t.TempDir(), 4, 1<<20)
	if _, err := m.Start("   ", ""); err == nil {
		t.Fatal("want error for empty command")
	}
}

func TestList(t *testing.T) {
	m := New(t.TempDir(), 4, 1<<20)
	a, err := m.Start("echo a", "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := m.Start("echo b", "")
	if err != nil {
		t.Fatal(err)
	}
	waitState(t, m, a.ID, StateFinished, 5*time.Second)
	waitState(t, m, b.ID, StateFinished, 5*time.Second)

	got, err := m.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("List len = %d, want 2", len(got))
	}
	if got[0].ID != b.ID {
		t.Fatalf("List newest first: got %s, want %s", got[0].ID, b.ID)
	}
	_ = m.Delete(a.ID)
	_ = m.Delete(b.ID)
}

// TestStaleMetaRepair simulates a server restart: a persisted task recorded as
// "running" whose process is gone must be repaired to "interrupted".
func TestStaleMetaRepair(t *testing.T) {
	m := New(t.TempDir(), 4, 1<<20)
	id := "t999999"
	dir := filepath.Join(m.dir, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	stale := TaskInfo{
		ID:        id,
		Command:   "old-task",
		State:     StateRunning,
		PID:       999999999, // almost certainly not alive
		StartedAt: time.Now().Add(-time.Hour),
	}
	if err := m.writeMetaInfo(stale); err != nil {
		t.Fatal(err)
	}
	info, err := m.Status(id)
	if err != nil {
		t.Fatal(err)
	}
	if info.State != StateInterrupted {
		t.Fatalf("state = %s, want interrupted (info: %+v)", info.State, info)
	}
	if err := m.Delete(id); err != nil {
		t.Fatal(err)
	}
}

func TestUnknownID(t *testing.T) {
	m := New(t.TempDir(), 4, 1<<20)
	if _, err := m.Status("t000000"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("status: err = %v, want ErrNotFound", err)
	}
	if _, err := m.Log("t000000", "all", 0); !errors.Is(err, ErrNotFound) {
		t.Fatalf("log: err = %v, want ErrNotFound", err)
	}
}
