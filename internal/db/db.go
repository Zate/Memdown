package db

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

var ErrNotFound = errors.New("not found")

type DB struct {
	db *sql.DB
}

// Open opens (or creates) the SQLite database at the given path.
func Open(path string) (*DB, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	sqlDB, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Set PRAGMAs explicitly (modernc driver doesn't support DSN query params)
	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA foreign_keys=ON",
	} {
		if _, err := sqlDB.Exec(pragma); err != nil {
			sqlDB.Close()
			return nil, fmt.Errorf("failed to execute %s: %w", pragma, err)
		}
	}

	d := &DB{db: sqlDB}
	if err := d.migrate(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	return d, nil
}

func (d *DB) Close() error {
	return d.db.Close()
}

func (d *DB) Exec(query string, args ...interface{}) (sql.Result, error) {
	return d.db.Exec(query, args...)
}

func (d *DB) QueryRow(query string, args ...interface{}) *sql.Row {
	return d.db.QueryRow(query, args...)
}

func (d *DB) Query(query string, args ...interface{}) (*sql.Rows, error) {
	return d.db.Query(query, args...)
}

func (d *DB) Begin() (*sql.Tx, error) {
	return d.db.Begin()
}

func (d *DB) getSchemaVersion() int {
	var version int
	err := d.db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_version").Scan(&version)
	if err != nil {
		return 0
	}
	return version
}

func (d *DB) setSchemaVersion(tx *sql.Tx, version int) error {
	_, err := tx.Exec("INSERT INTO schema_version (version, applied_at) VALUES (?, ?)",
		version, time.Now().UTC().Format(time.RFC3339))
	return err
}

var migrations = []struct {
	version int
	sqls    []string
}{
	{1, []string{
		`CREATE TABLE IF NOT EXISTS schema_version (
			version INTEGER PRIMARY KEY,
			applied_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS nodes (
			id TEXT PRIMARY KEY,
			type TEXT NOT NULL,
			content TEXT NOT NULL,
			summary TEXT,
			token_estimate INTEGER NOT NULL,
			superseded_by TEXT REFERENCES nodes(id),
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			metadata TEXT DEFAULT '{}'
		)`,
		`CREATE INDEX IF NOT EXISTS idx_nodes_type ON nodes(type)`,
		`CREATE INDEX IF NOT EXISTS idx_nodes_created ON nodes(created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_nodes_superseded ON nodes(superseded_by)`,
		`CREATE TABLE IF NOT EXISTS edges (
			id TEXT PRIMARY KEY,
			from_id TEXT NOT NULL,
			to_id TEXT NOT NULL,
			type TEXT NOT NULL,
			created_at TEXT NOT NULL,
			metadata TEXT DEFAULT '{}',
			FOREIGN KEY (from_id) REFERENCES nodes(id) ON DELETE CASCADE,
			FOREIGN KEY (to_id) REFERENCES nodes(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_edges_from ON edges(from_id)`,
		`CREATE INDEX IF NOT EXISTS idx_edges_to ON edges(to_id)`,
		`CREATE INDEX IF NOT EXISTS idx_edges_type ON edges(type)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_edges_unique ON edges(from_id, to_id, type)`,
		`CREATE TABLE IF NOT EXISTS tags (
			node_id TEXT NOT NULL,
			tag TEXT NOT NULL,
			created_at TEXT NOT NULL,
			PRIMARY KEY (node_id, tag),
			FOREIGN KEY (node_id) REFERENCES nodes(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_tags_tag ON tags(tag)`,
		`CREATE TABLE IF NOT EXISTS views (
			name TEXT PRIMARY KEY,
			query TEXT NOT NULL,
			budget INTEGER DEFAULT 50000,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS pending (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			created_at TEXT NOT NULL
		)`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS nodes_fts USING fts5(
			content,
			content='nodes',
			content_rowid='rowid'
		)`,
		`CREATE TRIGGER IF NOT EXISTS nodes_ai AFTER INSERT ON nodes BEGIN
			INSERT INTO nodes_fts(rowid, content) VALUES (NEW.rowid, NEW.content);
		END`,
		`CREATE TRIGGER IF NOT EXISTS nodes_ad AFTER DELETE ON nodes BEGIN
			INSERT INTO nodes_fts(nodes_fts, rowid, content) VALUES('delete', OLD.rowid, OLD.content);
		END`,
		`CREATE TRIGGER IF NOT EXISTS nodes_au AFTER UPDATE ON nodes BEGIN
			INSERT INTO nodes_fts(nodes_fts, rowid, content) VALUES('delete', OLD.rowid, OLD.content);
			INSERT INTO nodes_fts(rowid, content) VALUES (NEW.rowid, NEW.content);
		END`,
	}},
	{2, []string{
		// Update default view to exclude tier:reference from auto-loading
		`UPDATE views SET query = 'tag:tier:pinned OR tag:tier:working',
			updated_at = datetime('now')
			WHERE name = 'default'
			AND query = 'tag:tier:pinned OR tag:tier:reference OR tag:tier:working'`,
	}},
}

func (d *DB) migrate() error {
	// Ensure schema_version table exists first
	_, err := d.db.Exec(`CREATE TABLE IF NOT EXISTS schema_version (
		version INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`)
	if err != nil {
		return fmt.Errorf("failed to create schema_version table: %w", err)
	}

	currentVersion := d.getSchemaVersion()

	for _, m := range migrations {
		if m.version > currentVersion {
			tx, err := d.db.Begin()
			if err != nil {
				return fmt.Errorf("failed to begin transaction for migration %d: %w", m.version, err)
			}

			for _, s := range m.sqls {
				if _, err := tx.Exec(s); err != nil {
					_ = tx.Rollback()
					return fmt.Errorf("migration %d failed: %w", m.version, err)
				}
			}

			if err := d.setSchemaVersion(tx, m.version); err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("failed to set schema version %d: %w", m.version, err)
			}

			if err := tx.Commit(); err != nil {
				return fmt.Errorf("failed to commit migration %d: %w", m.version, err)
			}
		}
	}

	// Create default view if not exists
	_, err = d.db.Exec(`INSERT OR IGNORE INTO views (name, query, budget, created_at, updated_at)
		VALUES ('default', 'tag:tier:pinned OR tag:tier:working', 50000, ?, ?)`,
		time.Now().UTC().Format(time.RFC3339), time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("failed to create default view: %w", err)
	}

	return nil
}
