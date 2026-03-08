package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// SessionInfo is a lightweight summary of a saved session for listing.
type SessionInfo struct {
	Key     string    `json:"key"`
	Summary string    `json:"summary,omitempty"`
	Created time.Time `json:"created"`
	Updated time.Time `json:"updated"`
	Count   int       `json:"count"` // message count
}

// ListSessions returns summaries of all sessions, sorted by update time (newest first).
func (sm *SessionManager) ListSessions() []SessionInfo {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	infos := make([]SessionInfo, 0, len(sm.sessions))
	for _, s := range sm.sessions {
		infos = append(infos, SessionInfo{
			Key:     s.Key,
			Summary: s.Summary,
			Created: s.Created,
			Updated: s.Updated,
			Count:   len(s.Messages),
		})
	}

	sort.Slice(infos, func(i, j int) bool {
		return infos[i].Updated.After(infos[j].Updated)
	})

	return infos
}

// Delete removes a session from memory and deletes its file on disk.
func (sm *SessionManager) Delete(key string) error {
	sm.mu.Lock()
	delete(sm.sessions, key)
	sm.mu.Unlock()

	if sm.storage == "" {
		return nil
	}

	filename := sanitizeFilename(key)
	sessionPath := filepath.Join(sm.storage, filename+".json")
	if err := os.Remove(sessionPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// LoadSessionInfo reads a single session file and returns its info without
// loading it into the in-memory map. Useful for disk-only listing.
func LoadSessionInfo(path string) (*SessionInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s Session
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &SessionInfo{
		Key:     s.Key,
		Summary: s.Summary,
		Created: s.Created,
		Updated: s.Updated,
		Count:   len(s.Messages),
	}, nil
}
