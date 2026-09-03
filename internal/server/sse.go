// Package server – SSE (Server-Sent Events) helpers for the terminal stream.
package server

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
)

// sseWriter wraps an http.ResponseWriter for sending SSE events.
type sseWriter struct {
	w http.ResponseWriter
	f http.Flusher
}

func newSSEWriter(w http.ResponseWriter) (*sseWriter, bool) {
	f, ok := w.(http.Flusher)
	if !ok {
		return nil, false
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	return &sseWriter{w: w, f: f}, true
}

func (s *sseWriter) send(event, data string) error {
	_, err := fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", event, data)
	if err != nil {
		return err
	}
	s.f.Flush()
	return nil
}

// sendBytes sends raw (possibly binary) terminal bytes as an "output" event,
// base64-encoded on a single data: line so embedded newlines cannot break the
// SSE framing. The client decodes before rendering.
func (s *sseWriter) sendBytes(event string, data []byte) error {
	encoded := base64.StdEncoding.EncodeToString(data)
	_, err := fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", event, encoded)
	if err != nil {
		return err
	}
	s.f.Flush()
	return nil
}

// streamReader copies from r to the SSE writer, sending each chunk as an
// "output" event. It returns when r is closed or an error occurs.
func (s *sseWriter) streamChunk(r io.Reader) error {
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			if sendErr := s.sendBytes("output", buf[:n]); sendErr != nil {
				return sendErr
			}
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}
