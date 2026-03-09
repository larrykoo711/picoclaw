package agent

import (
	"sync"
	"testing"
)

// TestEmitToolEvent_StartCarriesArgs verifies that tool_start events include
// the tool call ID and argument map.
func TestEmitToolEvent_StartCarriesArgs(t *testing.T) {
	al, _, _, _, cleanup := newTestAgentLoop(t)
	defer cleanup()

	var received StreamChatEvent
	al.streamCallbacks.Store("sess1", func(evt StreamChatEvent) {
		received = evt
	})

	args := map[string]any{"command": "ls -la", "timeout": float64(30)}
	al.emitToolEvent("sess1", StreamChatEvent{
		Type:       "tool_start",
		Tool:       "exec",
		ToolCallID: "call_abc",
		ToolArgs:   args,
	})

	if received.Type != "tool_start" {
		t.Fatalf("Type = %q, want %q", received.Type, "tool_start")
	}
	if received.Tool != "exec" {
		t.Errorf("Tool = %q, want %q", received.Tool, "exec")
	}
	if received.ToolCallID != "call_abc" {
		t.Errorf("ToolCallID = %q, want %q", received.ToolCallID, "call_abc")
	}
	if received.ToolArgs["command"] != "ls -la" {
		t.Errorf("ToolArgs[command] = %v, want %q", received.ToolArgs["command"], "ls -la")
	}
	if received.ToolArgs["timeout"] != float64(30) {
		t.Errorf("ToolArgs[timeout] = %v, want 30", received.ToolArgs["timeout"])
	}
}

// TestEmitToolEvent_EndCarriesResultAndDuration verifies that tool_end events
// carry result preview and duration.
func TestEmitToolEvent_EndCarriesResultAndDuration(t *testing.T) {
	al, _, _, _, cleanup := newTestAgentLoop(t)
	defer cleanup()

	var received StreamChatEvent
	al.streamCallbacks.Store("sess1", func(evt StreamChatEvent) {
		received = evt
	})

	al.emitToolEvent("sess1", StreamChatEvent{
		Type:         "tool_end",
		Tool:         "read_file",
		ToolCallID:   "call_xyz",
		ToolResult:   "file content here...",
		ToolDuration: 180,
	})

	if received.Type != "tool_end" {
		t.Fatalf("Type = %q, want %q", received.Type, "tool_end")
	}
	if received.ToolCallID != "call_xyz" {
		t.Errorf("ToolCallID = %q, want %q", received.ToolCallID, "call_xyz")
	}
	if received.ToolResult != "file content here..." {
		t.Errorf("ToolResult = %q, want %q", received.ToolResult, "file content here...")
	}
	if received.ToolDuration != 180 {
		t.Errorf("ToolDuration = %d, want 180", received.ToolDuration)
	}
}

// TestEmitToolEvent_WildcardFallback verifies that when no per-session callback
// is registered, emitToolEvent falls back to the "*" wildcard.
func TestEmitToolEvent_WildcardFallback(t *testing.T) {
	al, _, _, _, cleanup := newTestAgentLoop(t)
	defer cleanup()

	var received StreamChatEvent
	al.streamCallbacks.Store("*", func(evt StreamChatEvent) {
		received = evt
	})

	al.emitToolEvent("unknown-session", StreamChatEvent{
		Type:       "tool_start",
		Tool:       "exec",
		ToolCallID: "call_wc",
		ToolArgs:   map[string]any{"command": "echo hi"},
	})

	if received.ToolCallID != "call_wc" {
		t.Errorf("wildcard fallback: ToolCallID = %q, want %q", received.ToolCallID, "call_wc")
	}
}

// TestEmitToolEvent_NoCallback verifies that emitToolEvent is a no-op when
// no callback is registered (no panic, no side effects).
func TestEmitToolEvent_NoCallback(t *testing.T) {
	al, _, _, _, cleanup := newTestAgentLoop(t)
	defer cleanup()

	// Should not panic
	al.emitToolEvent("nonexistent", StreamChatEvent{
		Type:       "tool_start",
		Tool:       "exec",
		ToolCallID: "call_noop",
	})
}

// TestEmitToolEvent_PairCorrelation verifies that tool_start and tool_end
// events for the same tool call share the same ToolCallID, enabling the
// frontend to correlate them.
func TestEmitToolEvent_PairCorrelation(t *testing.T) {
	al, _, _, _, cleanup := newTestAgentLoop(t)
	defer cleanup()

	var events []StreamChatEvent
	al.streamCallbacks.Store("sess1", func(evt StreamChatEvent) {
		events = append(events, evt)
	})

	callID := "call_pair_001"

	al.emitToolEvent("sess1", StreamChatEvent{
		Type:       "tool_start",
		Tool:       "exec",
		ToolCallID: callID,
		ToolArgs:   map[string]any{"command": "date"},
	})
	al.emitToolEvent("sess1", StreamChatEvent{
		Type:         "tool_end",
		Tool:         "exec",
		ToolCallID:   callID,
		ToolResult:   "Mon Mar 8 12:00:00 CST 2026",
		ToolDuration: 42,
	})

	if len(events) != 2 {
		t.Fatalf("events count = %d, want 2", len(events))
	}
	if events[0].Type != "tool_start" || events[1].Type != "tool_end" {
		t.Fatalf("event types = [%s, %s], want [tool_start, tool_end]",
			events[0].Type, events[1].Type)
	}
	if events[0].ToolCallID != events[1].ToolCallID {
		t.Errorf("ToolCallID mismatch: start=%q end=%q",
			events[0].ToolCallID, events[1].ToolCallID)
	}
	if events[0].ToolArgs["command"] != "date" {
		t.Errorf("start event missing args: %v", events[0].ToolArgs)
	}
	if events[1].ToolResult == "" {
		t.Error("end event missing result")
	}
	if events[1].ToolDuration != 42 {
		t.Errorf("end event duration = %d, want 42", events[1].ToolDuration)
	}
}

// TestEmitToolEvent_ConcurrentSessions verifies that concurrent emitToolEvent
// calls to different sessions are correctly routed without races.
func TestEmitToolEvent_ConcurrentSessions(t *testing.T) {
	al, _, _, _, cleanup := newTestAgentLoop(t)
	defer cleanup()

	var mu1, mu2 sync.Mutex
	var events1, events2 []StreamChatEvent

	al.streamCallbacks.Store("sess-a", func(evt StreamChatEvent) {
		mu1.Lock()
		events1 = append(events1, evt)
		mu1.Unlock()
	})
	al.streamCallbacks.Store("sess-b", func(evt StreamChatEvent) {
		mu2.Lock()
		events2 = append(events2, evt)
		mu2.Unlock()
	})

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			al.emitToolEvent("sess-a", StreamChatEvent{
				Type:       "tool_start",
				Tool:       "exec",
				ToolCallID: "a",
			})
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			al.emitToolEvent("sess-b", StreamChatEvent{
				Type:       "tool_start",
				Tool:       "read_file",
				ToolCallID: "b",
			})
		}
	}()
	wg.Wait()

	mu1.Lock()
	count1 := len(events1)
	mu1.Unlock()
	mu2.Lock()
	count2 := len(events2)
	mu2.Unlock()

	if count1 != 50 {
		t.Errorf("sess-a events = %d, want 50", count1)
	}
	if count2 != 50 {
		t.Errorf("sess-b events = %d, want 50", count2)
	}

	// Verify no cross-contamination
	mu1.Lock()
	for _, e := range events1 {
		if e.Tool != "exec" {
			t.Errorf("sess-a got tool %q from sess-b", e.Tool)
		}
	}
	mu1.Unlock()
	mu2.Lock()
	for _, e := range events2 {
		if e.Tool != "read_file" {
			t.Errorf("sess-b got tool %q from sess-a", e.Tool)
		}
	}
	mu2.Unlock()
}
