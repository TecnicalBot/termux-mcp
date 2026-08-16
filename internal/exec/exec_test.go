package exec

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestRunSuccess(t *testing.T) {
	res, err := Run(context.Background(), "sh", []string{"-c", "echo hi"}, Config{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d", res.ExitCode)
	}
	if strings.TrimSpace(res.Stdout) != "hi" {
		t.Fatalf("stdout = %q", res.Stdout)
	}
}

func TestRunExitCode(t *testing.T) {
	res, err := Run(context.Background(), "sh", []string{"-c", "exit 3"}, Config{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if res.ExitCode != 3 {
		t.Fatalf("exit = %d, want 3", res.ExitCode)
	}
}

func TestRunTimeout(t *testing.T) {
	res, err := Run(context.Background(), "sh", []string{"-c", "sleep 5"}, Config{Timeout: 100 * time.Millisecond})
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !res.TimedOut {
		t.Fatal("TimedOut flag not set")
	}
}

func TestRunOutputCap(t *testing.T) {
	res, err := Run(context.Background(), "sh", []string{"-c", "seq 1 100000"}, Config{Timeout: 10 * time.Second, MaxOutput: 100})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(res.Stdout) != 100+len(TruncatedMarker) {
		t.Fatalf("stdout len = %d, want %d", len(res.Stdout), 100+len(TruncatedMarker))
	}
	if !strings.Contains(res.Stdout, TruncatedMarker) {
		t.Fatal("missing truncation marker")
	}
}

func TestRunNotFound(t *testing.T) {
	_, err := Run(context.Background(), "definitely-not-a-command-xyz", nil, Config{Timeout: time.Second})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestRunJSONValid(t *testing.T) {
	raw, res, err := RunJSON(context.Background(), "sh", []string{"-c", `echo '{"a":1}'`},
		Config{Timeout: 5 * time.Second}, false)
	if err != nil {
		t.Fatalf("runjson: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d", res.ExitCode)
	}
	if string(raw) != `{"a":1}` {
		t.Fatalf("raw = %s", raw)
	}
}

func TestRunJSONInvalid(t *testing.T) {
	_, _, err := RunJSON(context.Background(), "sh", []string{"-c", "echo not-json"},
		Config{Timeout: 5 * time.Second}, false)
	if err == nil {
		t.Fatal("expected invalid JSON error")
	}
}

func TestRunJSONEmpty(t *testing.T) {
	_, _, err := RunJSON(context.Background(), "sh", []string{"-c", "true"},
		Config{Timeout: 5 * time.Second}, false)
	if err == nil {
		t.Fatal("expected empty output error")
	}
}
