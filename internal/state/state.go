package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// State holds the persistent collector state.
type State struct {
	LastUpdatedTime int64 `json:"last_updated_time"`
}

// Manager provides thread-safe state persistence.
type Manager struct {
	mu       sync.Mutex
	filePath string
	state    State
}

// NewManager creates a new state manager and loads existing state from disk.
func NewManager(filePath string) (*Manager, error) {
	m := &Manager{
		filePath: filePath,
	}
	if err := m.load(); err != nil {
		return nil, err
	}
	return m, nil
}

// GetLastUpdatedTime returns the last processed timestamp.
func (m *Manager) GetLastUpdatedTime() int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state.LastUpdatedTime
}

// SetLastUpdatedTime updates and persists the last processed timestamp.
func (m *Manager) SetLastUpdatedTime(t int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.LastUpdatedTime = t
	return m.save()
}

// load reads state from disk. Returns zero-state if file does not exist.
func (m *Manager) load() error {
	data, err := os.ReadFile(m.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			m.state = State{LastUpdatedTime: 0}
			return nil
		}
		return fmt.Errorf("reading state file: %w", err)
	}
	if err := json.Unmarshal(data, &m.state); err != nil {
		return fmt.Errorf("parsing state file: %w", err)
	}
	return nil
}

// save writes state to disk atomically (write to temp → rename).
func (m *Manager) save() error {
	data, err := json.MarshalIndent(&m.state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling state: %w", err)
	}

	dir := filepath.Dir(m.filePath)
	tmp, err := os.CreateTemp(dir, "state-*.json.tmp")
	if err != nil {
		return fmt.Errorf("creating temp state file: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("writing temp state file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("closing temp state file: %w", err)
	}
	if err := os.Rename(tmpName, m.filePath); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("renaming state file: %w", err)
	}
	return nil
}
