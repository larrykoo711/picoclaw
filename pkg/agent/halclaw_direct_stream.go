package agent

import (
	"context"
	"fmt"

	"github.com/sipeed/picoclaw/pkg/providers"
)

// ProcessDirectStream runs an agent loop iteration with streaming events.
// The callback receives high-level StreamChatEvent for desktop UI consumption.
// It wraps ProcessDirect, forwarding LLM streaming events via the callback.
func (al *AgentLoop) ProcessDirectStream(
	ctx context.Context,
	content, sessionKey string,
	callback func(StreamChatEvent),
) error {
	if callback == nil {
		callback = func(StreamChatEvent) {}
	}

	// Register callback under original sessionKey AND wildcard key.
	// ProcessDirect routes the message, and the loop's internal opts.SessionKey
	// may differ from the caller's sessionKey (e.g. "agent:main:main" vs "halclaw-...").
	// By also registering under "*", streamCallback can always find it.
	al.streamCallbacks.Store(sessionKey, callback)
	al.streamCallbacks.Store("*", callback)
	defer func() {
		al.streamCallbacks.Delete(sessionKey)
		al.streamCallbacks.Delete("*")
	}()

	reply, err := al.ProcessDirect(ctx, content, sessionKey)
	if err != nil {
		// Error is returned to the caller (app.go goroutine) which emits
		// chat:error. Do NOT also fire the callback here — that would cause
		// a double-emit (chat:limit + chat:error) where the first deletes
		// sessionMeta, preventing the second from finding the assistant message.
		return err
	}

	// Emit final "done" event. If ProcessDirect returned content that wasn't
	// streamed token-by-token (e.g. non-streaming provider fallback), emit it
	// as a single token event before done.
	_ = reply

	callback(StreamChatEvent{Type: "done"})
	return nil
}

// mapStreamEvent converts a low-level provider StreamEvent into a high-level
// StreamChatEvent for the desktop client. Returns nil for events that should
// not be forwarded.
func mapStreamEvent(event providers.StreamEvent) *StreamChatEvent {
	switch event.Type {
	case "content":
		if event.Content == "" {
			return nil
		}
		return &StreamChatEvent{
			Type:    "token",
			Content: event.Content,
		}

	case "thinking_start":
		return &StreamChatEvent{Type: "thinking_start"}

	case "reasoning":
		if event.ReasoningContent == "" {
			return nil
		}
		return &StreamChatEvent{
			Type:    "thinking_token",
			Content: event.ReasoningContent,
		}

	case "done":
		ev := &StreamChatEvent{Type: "done"}
		if event.Usage != nil {
			ev.InputTokens = event.Usage.PromptTokens
		}
		return ev

	case "error":
		msg := "unknown error"
		if event.Error != nil {
			msg = event.Error.Error()
		}
		return &StreamChatEvent{
			Type:    "error",
			Content: msg,
			Code:    classifyError(event.Error),
		}

	default:
		return nil
	}
}

// classifyError returns an i18n-friendly error code from an error.
func classifyError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()

	switch {
	case contains(msg, "rate_limit", "rate limit", "429"):
		return "rate_limit"
	case contains(msg, "context_length", "context_too_long", "maximum context"):
		return "context_too_long"
	case contains(msg, "timeout", "deadline"):
		return "timeout"
	case contains(msg, "unauthorized", "401", "invalid api key"):
		return "auth_error"
	default:
		return "unknown"
	}
}

func contains(s string, substrs ...string) bool {
	lower := fmt.Sprintf("%s", s)
	for _, sub := range substrs {
		if len(sub) > 0 && len(lower) >= len(sub) {
			for i := 0; i <= len(lower)-len(sub); i++ {
				if lower[i:i+len(sub)] == sub {
					return true
				}
			}
		}
	}
	return false
}
