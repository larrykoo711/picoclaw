package agent

// StreamChatEvent is a high-level streaming event for the desktop client.
// It translates low-level provider StreamEvents into UI-friendly types
// that map directly to Wails event emissions.
type StreamChatEvent struct {
	Type        string // "token", "thinking_start", "thinking_token", "tool_start", "tool_end", "done", "error"
	Content     string // text content for token/thinking_token/error
	Tool        string // tool name for tool_start/tool_end
	InputTokens int    // final input token count for "done"
	Code        string // error code for i18n ("rate_limit", "context_too_long", etc.)
}
