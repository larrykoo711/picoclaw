package agent

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/sipeed/picoclaw/pkg/providers"
	"github.com/sipeed/picoclaw/pkg/session"
)

// SetIdentity sets the agent's display name and emoji for the desktop client.
func (al *AgentLoop) SetIdentity(name, emoji string) {
	al.identityName = name
	al.identityEmoji = emoji
}

// SetMemoryStatusCallback registers a callback invoked when background
// summarization starts ("start") or completes ("done") for a session.
func (al *AgentLoop) SetMemoryStatusCallback(cb func(sessionKey, status string)) {
	al.memoryStatusCb = cb
}

// SetDisabledSkills sets the list of skills to exclude from the system prompt.
// Propagates to all agent ContextBuilders so the next prompt build filters them out.
func (al *AgentLoop) SetDisabledSkills(names []string) {
	al.disabledSkills = names
	// Sync to every registered agent's ContextBuilder.
	for _, id := range al.registry.ListAgentIDs() {
		if agent, ok := al.registry.GetAgent(id); ok && agent.ContextBuilder != nil {
			agent.ContextBuilder.SetDisabledSkills(names)
		}
	}
}

// activeSessionManager returns the session manager from the running agent
// (which always reflects the latest in-memory state), falling back to
// a disk-loaded session manager when no agent is running.
func (al *AgentLoop) activeSessionManager() *session.SessionManager {
	// Prefer the live agent's session manager — it has the real-time state
	// that AddMessage/Save keep up to date during message processing.
	if agent := al.registry.GetDefaultAgent(); agent != nil && agent.Sessions != nil {
		return agent.Sessions
	}
	// Fallback: lazily load from disk (engine starting or no agents registered).
	al.sessionMgrOnce.Do(func() {
		workspace := al.cfg.Agents.Defaults.Workspace
		if workspace == "" {
			workspace = "~/.picoclaw/workspace"
		}
		workspace = expandHome(workspace)
		sessDir := filepath.Join(workspace, "sessions")
		al.sessionMgr = session.NewSessionManager(sessDir)
	})
	return al.sessionMgr
}

// ListSessions returns summaries of all saved sessions.
func (al *AgentLoop) ListSessions() []session.SessionInfo {
	sm := al.activeSessionManager()
	if sm == nil {
		return nil
	}
	return sm.ListSessions()
}

// DeleteSession removes a session by key from both memory and disk.
func (al *AgentLoop) DeleteSession(key string) error {
	sm := al.activeSessionManager()
	if sm == nil {
		return fmt.Errorf("session manager not available")
	}
	return sm.Delete(key)
}

// GetHistory returns the message history for a session.
func (al *AgentLoop) GetHistory(key string) []providers.Message {
	sm := al.activeSessionManager()
	if sm == nil {
		return nil
	}
	return sm.GetHistory(key)
}

// TruncateSessionHistory truncates a session, keeping only the first
// keepUserCount user messages and their corresponding assistant/tool responses.
// keepUserCount=0 clears all history.
func (al *AgentLoop) TruncateSessionHistory(key string, keepUserCount int) {
	sm := al.activeSessionManager()
	if sm == nil {
		return
	}

	if keepUserCount <= 0 {
		sm.TruncateHistory(key, 0)
		_ = sm.Save(key)
		return
	}

	// Find the message index after the Nth user message's complete turn.
	history := sm.GetHistory(key)
	usersSeen := 0
	cutIndex := len(history)
	for i, msg := range history {
		if msg.Role == "user" {
			usersSeen++
			if usersSeen > keepUserCount {
				cutIndex = i
				break
			}
		}
	}

	if cutIndex < len(history) {
		sm.SetHistory(key, history[:cutIndex])
		_ = sm.Save(key)
	}
}

// ForceSavePartialSession appends partial content as an assistant message
// and saves the session. Used during app shutdown when a stream is in progress.
func (al *AgentLoop) ForceSavePartialSession(sessionKey, partialContent string) error {
	sm := al.activeSessionManager()
	if sm == nil {
		return fmt.Errorf("session manager not available")
	}

	if partialContent != "" {
		sm.AddMessage(sessionKey, "assistant", partialContent)
	}
	return sm.Save(sessionKey)
}

// EstimateSessionTokens returns a fast local heuristic token estimate
// for the session's message history. Never makes a network request.
func (al *AgentLoop) EstimateSessionTokens(sessionKey string) int {
	sm := al.activeSessionManager()
	if sm == nil {
		return 0
	}
	messages := sm.GetHistory(sessionKey)
	return estimateMessagesTokens(messages)
}

// CountSessionTokens calls the provider's token counting API for the session.
// Falls back to local estimation if the provider does not support token counting.
func (al *AgentLoop) CountSessionTokens(ctx context.Context, sessionKey string) (int, error) {
	sm := al.activeSessionManager()
	if sm == nil {
		return 0, fmt.Errorf("session manager not available")
	}

	messages := sm.GetHistory(sessionKey)

	// Get the default agent's provider for token counting
	agent := al.registry.GetDefaultAgent()
	if agent == nil {
		return estimateMessagesTokens(messages), nil
	}

	counter, ok := agent.Provider.(providers.TokenCounterProvider)
	if !ok {
		return estimateMessagesTokens(messages), nil
	}

	return counter.CountTokens(ctx, messages, nil, agent.Model)
}
