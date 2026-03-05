package protocoltypes

import (
	"bufio"
	"io"
	"strings"
)

// ParseSSEStream reads an SSE stream from r and emits each "data:" payload
// on the returned channel. The channel is closed when the stream ends
// (EOF or [DONE] marker). Comment lines (starting with ':') and empty
// lines are silently ignored.
//
// The caller should cancel the context or close the reader to stop parsing;
// the goroutine will exit without leaking.
func ParseSSEStream(r io.Reader) <-chan string {
	ch := make(chan string, 16)
	go func() {
		defer close(ch)
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			line := scanner.Text()

			// Skip empty lines and comment/keepalive lines.
			if line == "" || strings.HasPrefix(line, ":") {
				continue
			}

			// Only process "data:" lines.
			if !strings.HasPrefix(line, "data:") {
				continue
			}

			payload := strings.TrimPrefix(line, "data:")
			payload = strings.TrimSpace(payload)

			// [DONE] is the standard SSE termination marker.
			if payload == "[DONE]" {
				return
			}

			if payload != "" {
				ch <- payload
			}
		}
	}()
	return ch
}
