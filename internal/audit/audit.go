// Package audit writes a JSONL audit trail of every tool call with argument
// redaction, so sensitive values never reach the log.
package audit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Entry is one audited tool invocation.
type Entry struct {
	Time       time.Time      `json:"time"`
	Caller     string         `json:"caller"`
	Tool       string         `json:"tool"`
	Args       map[string]any `json:"args"`
	DurationMS int64          `json:"duration_ms"`
	ExitCode   int            `json:"exit_code"`
	Error      string         `json:"error,omitempty"`
}

// Logger appends JSON lines to an audit file (created on demand).
type Logger struct {
	mu     sync.Mutex
	path   string
	file   *os.File
	closed bool
}

// New opens (creating if needed) the audit file at path.
func New(path string) (*Logger, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("audit dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("audit file: %w", err)
	}
	return &Logger{path: path, file: f}, nil
}

// Disabled returns a no-op logger.
func Disabled() *Logger { return &Logger{closed: true} }

// Log writes one entry. Args are redacted and deep-copied before writing.
func (l *Logger) Log(e Entry) error {
	if l == nil || l.closed {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	e.Args = Redact(e.Args)
	line, err := json.Marshal(e)
	if err != nil {
		return err
	}
	_, err = l.file.Write(append(line, '\n'))
	return err
}

// Close flushes and closes the underlying file.
func (l *Logger) Close() error {
	if l == nil || l.closed {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.closed = true
	return l.file.Close()
}

// sensitiveKeys match argument names whose values must never be logged.
var sensitiveKeys = []string{
	"token", "password", "secret", "apikey", "api_key", "key",
	"pin", "otp", "authorization", "content", "text", "message", "body",
}

const maxValueLen = 200

// Redact returns a copy of args with sensitive or oversized values scrubbed.
func Redact(args map[string]any) map[string]any {
	out := make(map[string]any, len(args))
	for k, v := range args {
		lk := strings.ToLower(k)
		sensitive := false
		for _, sk := range sensitiveKeys {
			if strings.Contains(lk, sk) {
				sensitive = true
				break
			}
		}
		switch val := v.(type) {
		case string:
			if sensitive {
				out[k] = "[REDACTED]"
			} else if len(val) > maxValueLen {
				out[k] = val[:maxValueLen] + "...(truncated)"
			} else {
				out[k] = val
			}
		default:
			if sensitive {
				out[k] = "[REDACTED]"
			} else {
				out[k] = v
			}
		}
	}
	return out
}
