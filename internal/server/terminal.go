// Package server – terminal streaming handler: tails a background task's
// stdout.log and streams it to the client as Server-Sent Events.
package server

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"termux-mcp/internal/tasks"
)

// TerminalStreamHandler returns an http.Handler for GET /terminal/stream.
// Query params:
//
//	task_id  – required, e.g. "t000001"
//	stream   – "stdout" (default) or "all"
func TerminalStreamHandler(mgr *tasks.Manager, taskDir string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		taskID := r.URL.Query().Get("task_id")
		if taskID == "" {
			http.Error(w, `{"error":"missing task_id"}`, http.StatusBadRequest)
			return
		}

		sse, ok := newSSEWriter(w)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}

		info, err := mgr.Status(taskID)
		if err != nil {
			_ = sse.send("error", `{"message":"task not found"}`)
			return
		}

		stdoutPath := filepath.Join(taskDir, taskID, "stdout.log")
		f, err := os.Open(stdoutPath)
		if err != nil {
			_ = sse.send("error", `{"message":"log file not available"}`)
			return
		}
		defer f.Close()

		// Send initial task info.
		initMsg, _ := json.Marshal(map[string]any{
			"task_id":  info.ID,
			"command":  info.Command,
			"state":    info.State,
			"pid":      info.PID,
		})
		_ = sse.send("init", string(initMsg))

		// If the task already finished, send whatever is in the log and exit.
		if info.State != tasks.StateRunning {
			if data, err := io.ReadAll(f); err == nil && len(data) > 0 {
				_ = sse.sendBytes("output", data)
			}
			finalMsg, _ := json.Marshal(map[string]any{
				"type":      "done",
				"exit_code": info.ExitCode,
				"state":     info.State,
			})
			_ = sse.send("done", string(finalMsg))
			return
		}

		// Tail the log file until the task finishes.
		pollInterval := 100 * time.Millisecond
		ticker := time.NewTicker(pollInterval)
		defer ticker.Stop()

		seen := int64(0)
		for {
			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
				// Read any new bytes.
				stat, err := f.Stat()
				if err != nil {
					continue
				}
				size := stat.Size()
				if size > seen {
					buf := make([]byte, size-seen)
					n, err := f.ReadAt(buf, seen)
					if n > 0 {
						seen += int64(n)
						_ = sse.sendBytes("output", buf[:n])
					}
					if err != nil && err != io.EOF {
						slog.Warn("terminal stream read error", "err", err)
					}
				}

				// Check if the task finished.
				info, err = mgr.Status(taskID)
				if err != nil {
					return
				}
				if info.State != tasks.StateRunning {
					// Drain any remaining bytes.
					stat, err = f.Stat()
					if err == nil && stat.Size() > seen {
						buf := make([]byte, stat.Size()-seen)
						n, _ := f.ReadAt(buf, seen)
						if n > 0 {
							_ = sse.sendBytes("output", buf[:n])
						}
					}
					finalMsg, _ := json.Marshal(map[string]any{
						"type":      "done",
						"exit_code": info.ExitCode,
						"state":     info.State,
						"duration":  info.DurationMS,
					})
					_ = sse.send("done", string(finalMsg))
					return
				}
			}
		}
	})
}
