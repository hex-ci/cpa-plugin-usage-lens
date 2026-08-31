// store.go: SQLite persistence. Single *sql.DB handle (safe for concurrent
// use) opened against the configured DBPath; schema idempotent.
package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	_ "modernc.org/sqlite"
)

var (
	dbOnce   sync.Once
	dbHandle *sql.DB
	dbErr    error
)

const schema = `
CREATE TABLE IF NOT EXISTS usage_events (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	ts INTEGER NOT NULL,
	provider TEXT, model TEXT, alias TEXT, api_key TEXT,
	auth_id TEXT, auth_index TEXT, auth_type TEXT, source TEXT,
	latency_ms INTEGER, ttft_ms INTEGER,
	failed INTEGER NOT NULL DEFAULT 0, status_code INTEGER,
	input_tokens INTEGER, output_tokens INTEGER, reasoning_tokens INTEGER,
	cached_tokens INTEGER, total_tokens INTEGER,
	raw TEXT
);
CREATE INDEX IF NOT EXISTS idx_events_ts ON usage_events(ts);
CREATE INDEX IF NOT EXISTS idx_events_model ON usage_events(model);
CREATE INDEX IF NOT EXISTS idx_events_key ON usage_events(api_key);

CREATE TABLE IF NOT EXISTS model_pricing (
	model TEXT PRIMARY KEY,
	input_price REAL NOT NULL DEFAULT 0,
	output_price REAL NOT NULL DEFAULT 0,
	source TEXT,
	updated_at INTEGER
);

CREATE TABLE IF NOT EXISTS api_key_aliases (
	api_key TEXT PRIMARY KEY,
	alias TEXT,
	updated_at INTEGER
);
`

func openDB() (*sql.DB, error) {
	dbOnce.Do(func() {
		cfgMu.RLock()
		path := cfg.DBPath
		cfgMu.RUnlock()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			dbErr = err
			return
		}
		db, err := sql.Open("sqlite", path)
		if err != nil {
			dbErr = err
			return
		}
		db.SetMaxOpenConns(1) // single writer simplifies SQLite locking
		if _, err := db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
			dbErr = err
			return
		}
		if _, err := db.Exec(`PRAGMA busy_timeout=5000`); err != nil {
			dbErr = err
			return
		}
		if _, err := db.Exec(schema); err != nil {
			dbErr = err
			return
		}
		// 旧库迁移:model_pricing 补 source 列(models.dev 同步来源标记)。
		if _, err := db.Exec(`ALTER TABLE model_pricing ADD COLUMN source TEXT`); err != nil {
			// duplicate column name => 已存在,忽略
		}
		dbHandle = db
		dbErr = nil
	})
	return dbHandle, dbErr
}

// mustDB returns the singleton handle; callers treat a nil return as a fatal
// misconfiguration (logged, not panicked).
func mustDB() *sql.DB {
	db, err := openDB()
	if err != nil {
		fmt.Printf("[usage-lens] db open: %v\n", err)
		return nil
	}
	return db
}
