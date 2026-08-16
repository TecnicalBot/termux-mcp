// Package exec wraps os/exec with the safety properties the server needs:
// per-call timeout, capped output, controlled env, exit-code reporting, and
// retry-once JSON validation for flaky termux-api binaries.
package exec

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	osexec "os/exec"
	"strings"
	"time"
)

// TruncatedMarker is appended when output exceeds the configured cap.
const TruncatedMarker = "\n...[output truncated]"

// ErrNotFound wraps osexec.ErrNotFound with a hint.
var ErrNotFound = errors.New("command not found (is it installed?)")

// Config controls a single command invocation.
type Config struct {
	Timeout   time.Duration
	MaxOutput int64
	Env       []string
	Dir       string
}

// Result captures the outcome of a command.
type Result struct {
	Stdout   string        `json:"stdout"`
	Stderr   string        `json:"stderr"`
	ExitCode int           `json:"exit_code"`
	Duration time.Duration `json:"duration_ns"`
	TimedOut bool          `json:"timed_out"`
}

// LookPath reports whether a binary is on PATH.
func LookPath(name string) (string, error) { return osexec.LookPath(name) }

// CommandExists is a convenience wrapper around LookPath.
func CommandExists(name string) bool {
	_, err := osexec.LookPath(name)
	return err == nil
}

// Run executes name with args under cfg. A non-zero exit code is not an error
// here: it is reported in Result.ExitCode. Errors are returned for missing
// binaries, timeouts, and launch failures.
func Run(ctx context.Context, name string, args []string, cfg Config) (*Result, error) {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.MaxOutput <= 0 {
		cfg.MaxOutput = 1 << 20
	}
	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	cmd := osexec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), cfg.Env...)
	if cfg.Dir != "" {
		cmd.Dir = cfg.Dir
	}

	var stdout, stderr limitedBuffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err := cmd.Run()
	res := &Result{
		Stdout:   stdout.String(cfg.MaxOutput),
		Stderr:   stderr.String(cfg.MaxOutput),
		Duration: time.Since(start),
	}

	if ctx.Err() == context.DeadlineExceeded {
		res.TimedOut = true
		return res, fmt.Errorf("command %s timed out after %s", name, cfg.Timeout)
	}
	if err != nil {
		var ee *osexec.ExitError
		if errors.As(err, &ee) {
			res.ExitCode = ee.ExitCode()
			return res, nil
		}
		if errors.Is(err, osexec.ErrNotFound) {
			return res, fmt.Errorf("%w: %s", ErrNotFound, name)
		}
		return res, err
	}
	return res, nil
}

// RunJSON runs a command expected to emit a single JSON document on stdout,
// validates it, and returns the raw message. When retry is true, a failed or
// empty run is retried once (termux-api can return placeholders on first call).
func RunJSON(ctx context.Context, name string, args []string, cfg Config, retry bool) (json.RawMessage, *Result, error) {
	attempt := func() (json.RawMessage, *Result, error) {
		res, err := Run(ctx, name, args, cfg)
		if err != nil {
			return nil, res, err
		}
		if res.ExitCode != 0 {
			return nil, res, fmt.Errorf("%s exited %d: %s", name, res.ExitCode, strings.TrimSpace(res.Stderr))
		}
		out := strings.TrimSpace(res.Stdout)
		if out == "" {
			return nil, res, fmt.Errorf("%s returned empty output", name)
		}
		var raw json.RawMessage
		if err := json.Unmarshal([]byte(out), &raw); err != nil {
			return nil, res, fmt.Errorf("%s returned invalid JSON: %v", name, err)
		}
		return raw, res, nil
	}

	raw, res, err := attempt()
	if err != nil && retry {
		if raw2, res2, err2 := attempt(); err2 == nil {
			return raw2, res2, nil
		}
	}
	return raw, res, err
}

// limitedBuffer is a pre-truncation buffer with a hard cap so runaway output
// cannot exhaust memory before the configured cap is applied.
type limitedBuffer struct {
	b bytes.Buffer
}

const hardCap = 2 << 20 // 2 MiB

func (l *limitedBuffer) Write(p []byte) (int, error) {
	if l.b.Len() >= hardCap {
		return len(p), nil
	}
	room := hardCap - l.b.Len()
	if len(p) > room {
		p = p[:room]
	}
	return l.b.Write(p)
}

func (l *limitedBuffer) String(max int64) string {
	s := l.b.String()
	if int64(len(s)) > max {
		s = s[:max] + TruncatedMarker
	}
	return s
}
