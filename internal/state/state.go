package state

import (
	"database/sql"
	"fmt"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// Manager provides thread-safe state persistence using SQLite.
type Manager struct {
	mu sync.Mutex
	db *sql.DB
}

// NewManager creates a new SQLite state manager and initializes tables.
func NewManager(dbPath string) (*Manager, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite check: %w", err)
	}

	m := &Manager{db: db}
	if err := m.initDB(); err != nil {
		return nil, err
	}

	return m, nil
}

// initDB creates the necessary schema.
func (m *Manager) initDB() error {
	schema := `
	CREATE TABLE IF NOT EXISTS state (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		last_updated_time INTEGER NOT NULL
	);
	
	CREATE TABLE IF NOT EXISTS audit_log (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		offense_id INTEGER NOT NULL,
		status TEXT NOT NULL,
		error_message TEXT,
		payload TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	
	INSERT OR IGNORE INTO state (id, last_updated_time) VALUES (1, 0);
	`
	_, err := m.db.Exec(schema)
	if err != nil {
		return fmt.Errorf("initializing db schema: %w", err)
	}
	return nil
}

// GetLastUpdatedTime returns the highest processed timestamp.
func (m *Manager) GetLastUpdatedTime() int64 {
	m.mu.Lock()
	defer m.mu.Unlock()

	var t int64
	err := m.db.QueryRow("SELECT last_updated_time FROM state WHERE id = 1").Scan(&t)
	if err != nil {
		return 0
	}
	return t
}

// SetLastUpdatedTime safely updates the globally highest timestamp.
func (m *Manager) SetLastUpdatedTime(t int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, err := m.db.Exec("UPDATE state SET last_updated_time = ? WHERE id = 1", t)
	return err
}

// RecordAudit adds an attempt (success or error) to the detailed audit log.
func (m *Manager) RecordAudit(offenseID int64, status, errMsg, payload string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, err := m.db.Exec(
		"INSERT INTO audit_log (offense_id, status, error_message, payload, created_at) VALUES (?, ?, ?, ?, ?)",
		offenseID, status, errMsg, payload, time.Now().Format(time.RFC3339),
	)
	return err
}

// HasOffense checks if the offense has already been processed and successfully sent.
func (m *Manager) HasOffense(offenseID int64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	var exists bool
	query := "SELECT EXISTS(SELECT 1 FROM audit_log WHERE offense_id = ? AND status = 'SUCCESS' LIMIT 1)"
	err := m.db.QueryRow(query, offenseID).Scan(&exists)
	if err != nil {
		return false
	}
	return exists
}

// Close closes the database connection.
func (m *Manager) Close() error {
	return m.db.Close()
}
