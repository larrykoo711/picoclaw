package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/sipeed/picoclaw/pkg/providers"
)

// StreamCallback is called for each streaming event during an LLM call.
// The desktop client subscribes to this via the message bus.
type StreamCallback func(event providers.StreamEvent)

// streamLLMCall attempts to use streaming if the provider supports it,
// otherwise falls back to the non-streaming Chat method.
// It accumulates all streaming events into a complete LLMResponse.
func streamLLMCall(
	ctx context.Context,
	provider providers.LLMProvider,
	messages []providers.Message,
	tools []providers.ToolDefinition,
	model string,
	options map[string]any,
	callback StreamCallback,
) (*providers.LLMResponse, error) {
	// Try streaming if provider supports it
	sp, ok := provider.(providers.StreamingProvider)
	if !ok {
		// Fallback to non-streaming
		return provider.Chat(ctx, messages, tools, model, options)
	}

	ch, err := sp.ChatStream(ctx, messages, tools, model, options)
	if err != nil {
		return nil, err
	}

	return accumulateStream(ctx, ch, callback)
}

// accumulateStream reads all events from a streaming channel and builds
// a complete LLMResponse. Each event is also forwarded to the callback.
func accumulateStream(
	ctx context.Context,
	ch <-chan providers.StreamEvent,
	callback StreamCallback,
) (*providers.LLMResponse, error) {
	var content strings.Builder
	var reasoning strings.Builder
	var toolCalls []providers.ToolCall
	var finishReason string
	var usage *providers.UsageInfo
	var lastErr error

	// Track tool call accumulation by index
	toolCallMap := make(map[int]*toolCallAccumulator)

	for event := range ch {
		// Forward to callback
		if callback != nil {
			callback(event)
		}

		switch event.Type {
		case "content":
			content.WriteString(event.Content)

		case "reasoning":
			reasoning.WriteString(event.ReasoningContent)

		case "tool_call":
			if event.ToolCallDelta != nil {
				tc := event.ToolCallDelta
				acc, exists := toolCallMap[tc.Index]
				if !exists {
					acc = &toolCallAccumulator{Index: tc.Index}
					toolCallMap[tc.Index] = acc
				}
				if tc.ID != "" {
					acc.ID = tc.ID
				}
				if tc.Name != "" {
					acc.Name = tc.Name
				}
				if tc.Arguments != "" {
					acc.Arguments.WriteString(tc.Arguments)
				}
			}

		case "done":
			if event.FinishReason != "" {
				finishReason = event.FinishReason
			}
			if event.Usage != nil {
				usage = event.Usage
			}

		case "error":
			if event.Error != nil {
				lastErr = event.Error
			}
		}
	}

	if lastErr != nil {
		return nil, fmt.Errorf("stream error: %w", lastErr)
	}

	// Convert accumulated tool calls.
	// Iterate by actual keys because Anthropic content block indices may not
	// start at 0 (e.g. text=0, tool_use=1 → toolCallMap only has key 1).
	// Collect and sort indices to preserve order.
	indices := make([]int, 0, len(toolCallMap))
	for idx := range toolCallMap {
		indices = append(indices, idx)
	}
	sort.Ints(indices)
	for _, idx := range indices {
		acc := toolCallMap[idx]
		args := make(map[string]any)
		argsStr := acc.Arguments.String()
		if argsStr != "" {
			if err := json.Unmarshal([]byte(argsStr), &args); err != nil {
				args["raw"] = argsStr
			}
		}
		toolCalls = append(toolCalls, providers.ToolCall{
			ID:        acc.ID,
			Name:      acc.Name,
			Arguments: args,
		})
	}

	if finishReason == "" {
		finishReason = "stop"
	}

	return &providers.LLMResponse{
		Content:          content.String(),
		ReasoningContent: reasoning.String(),
		ToolCalls:        toolCalls,
		FinishReason:     finishReason,
		Usage:            usage,
	}, nil
}

// toolCallAccumulator collects streaming fragments of a single tool call.
type toolCallAccumulator struct {
	Index     int
	ID        string
	Name      string
	Arguments strings.Builder
}

// emitToolEvent sends a tool_start or tool_end StreamChatEvent to the registered callback.
func (al *AgentLoop) emitToolEvent(sessionKey string, evt StreamChatEvent) {
	cb, ok := al.streamCallbacks.Load(sessionKey)
	if !ok {
		cb, ok = al.streamCallbacks.Load("*")
		if !ok {
			return
		}
	}
	cb.(func(StreamChatEvent))(evt)
}

// streamCallback returns a StreamCallback for the current agent loop iteration.
// If a per-session StreamChatEvent callback is registered (via ProcessDirectStream),
// it converts low-level StreamEvents into high-level StreamChatEvents and forwards them.
func (al *AgentLoop) streamCallback(ctx context.Context, opts processOptions) StreamCallback {
	return func(event providers.StreamEvent) {
		cb, ok := al.streamCallbacks.Load(opts.SessionKey)
		if !ok {
			// Fallback: ProcessDirectStream registers under "*" because
			// the routed SessionKey may differ from the caller's key.
			cb, ok = al.streamCallbacks.Load("*")
			if !ok {
				return
			}
		}
		chatEvent := mapStreamEvent(event)
		// Skip "done" and "error" events from individual LLM calls.
		// - "done": ProcessDirectStream emits its own after the full agent loop.
		// - "error": intermediate errors (e.g. one LLM iteration failing) must not
		//   reach the frontend, which would prematurely set "limit" status and
		//   delete sessionMeta, causing all subsequent tokens to be silently dropped.
		//   ProcessDirectStream handles the final error at the top level.
		if chatEvent != nil && chatEvent.Type != "done" && chatEvent.Type != "error" {
			cb.(func(StreamChatEvent))(*chatEvent)
		}
	}
}
