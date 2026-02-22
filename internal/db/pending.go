package db

import (
	"database/sql"
	"fmt"
	"time"
)

func (d *SQLiteStore) SetPending(key, value string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := d.db.Exec(`INSERT OR REPLACE INTO pending (key, value, created_at) VALUES (?, ?, ?)`,
		key, value, now)
	if err != nil {
		return fmt.Errorf("failed to set pending %s: %w", key, err)
	}
	return nil
}

func (d *SQLiteStore) GetPending(key string) (string, error) {
	var value string
	err := d.db.QueryRow("SELECT value FROM pending WHERE key = ?", key).Scan(&value)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("failed to get pending %s: %w", key, err)
	}
	return value, nil
}

func (d *SQLiteStore) DeletePending(key string) error {
	_, err := d.db.Exec("DELETE FROM pending WHERE key = ?", key)
	return err
}
