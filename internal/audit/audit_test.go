package audit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRedact(t *testing.T) {
	in := map[string]any{
		"number":  "12345678",
		"text":    "secret message body",
		"token":   "abcdef",
		"safe":    "hello",
		"content": "long content",
	}
	out := Redact(in)
	if out["text"] != "[REDACTED]" {
		t.Fatalf("text not redacted: %v", out["text"])
	}
	if out["token"] != "[REDACTED]" {
		t.Fatalf("token not redacted: %v", out["token"])
	}
	if out["content"] != "[REDACTED]" {
		t.Fatalf("content not redacted: %v", out["content"])
	}
	if out["number"] != "12345678" {
		t.Fatalf("number wrongly redacted: %v", out["number"])
	}
	if out["safe"] != "hello" {
		t.Fatalf("safe value changed: %v", out["safe"])
	}
}

func TestRedactTruncateLong(t *testing.T) {
	long := strings.Repeat("x", 500)
	out := Redact(map[string]any{"note": long})
	v, ok := out["note"].(string)
	if !ok || len(v) > maxValueLen+20 {
		t.Fatalf("long value not truncated: %d", len(v))
	}
}

func TestLogAndRead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")
	l, err := New(path)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	defer l.Close()

	if err := l.Log(Entry{
		Tool: "send_sms",
		Args: map[string]any{"number": "123", "text": "hi there"},
	}); err != nil {
		t.Fatalf("log: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var e Entry
	if err := json.Unmarshal(data, &e); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if e.Tool != "send_sms" {
		t.Fatalf("tool = %q", e.Tool)
	}
	if e.Args["text"] != "[REDACTED]" {
		t.Fatalf("args not redacted on disk: %v", e.Args)
	}
	if e.Args["number"] != "123" {
		t.Fatalf("number lost: %v", e.Args)
	}
}

func TestDisabled(t *testing.T) {
	l := Disabled()
	if err := l.Log(Entry{Tool: "x"}); err != nil {
		t.Fatalf("disabled log: %v", err)
	}
	if err := l.Close(); err != nil {
		t.Fatalf("disabled close: %v", err)
	}
}
