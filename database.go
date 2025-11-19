// Package main provides functionalities for database interactions.
package main

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Activity represents a record from the activity table.
type Activity struct {
	Call    string
	Created time.Time
	ID      string
	Module  string
	System  string
	Updated time.Time
	Via     string
	Ts      float64
	Tsoff   int64
}

// ErrDBNotFound is returned when the database file is not found and creation is not requested.
var ErrDBNotFound = errors.New("database file not found")

// initDB initializes the SQLite database. If create is false, it requires the
// database file to already exist. If create is true, it will create the file
// and the necessary schema.
func initDB(dbPath string, create bool) (*sql.DB, error) {
	// Check if the database file already exists.
	_, err := os.Stat(dbPath)
	if os.IsNotExist(err) {
		if !create {
			return nil, ErrDBNotFound
		}

		slog.Info("Database not found, creating new one.", "path", dbPath)

		// Create the directory if it doesn't exist.
		dir := filepath.Dir(dbPath)
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			slog.Info("Creating database directory", "path", dir)
			if err := os.MkdirAll(dir, 0755); err != nil {
				return nil, err
			}
		}

		// Create and open the new database file.
		db, err := sql.Open("sqlite3", dbPath)
		if err != nil {
			return nil, err
		}

		// Create the schema for the new database.
		slog.Info("Creating database schema")
		createTableSQL := `CREATE TABLE activity (
			call TEXT DEFAULT '',
			created TEXT DEFAULT '' NOT NULL,
			id TEXT PRIMARY KEY NOT NULL,
			module TEXT DEFAULT '',
			system TEXT DEFAULT '',
			updated TEXT DEFAULT '' NOT NULL,
			via TEXT DEFAULT '',
			ts REAL DEFAULT 0,
			tsoff NUMERIC DEFAULT 0
		);`
		createIndexSQL := `CREATE INDEX callmodts ON activity (
			call,
			module,
			tsoff
		);`

		if _, err := db.Exec(createTableSQL); err != nil {
			return nil, err
		}
		if _, err := db.Exec(createIndexSQL); err != nil {
			return nil, err
		}

		slog.Info("Database initialized successfully")
		return db, nil
	}

	// If the file already exists, just open it.
	slog.Info("Opening existing database", "path", dbPath)
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}
	return db, nil
}

// getLastTime retrieves the timestamp of the most recent activity record for a given system.
func getLastTime(ctx context.Context, db *sql.DB, system string) (int64, error) {
	var ts float64
	err := db.QueryRowContext(ctx, "SELECT ts FROM activity WHERE system = ? ORDER BY ts DESC LIMIT 1", system).Scan(&ts)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, err
	}
	return int64(ts), nil
}

// saveActivity saves an activity record to the database.
func saveActivity(ctx context.Context, db *sql.DB, activity Activity) error {
	_, err := db.ExecContext(ctx, `
        INSERT INTO activity (id, call, created, module, system, updated, via, ts, tsoff)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
    `, activity.ID, activity.Call, activity.Created, activity.Module, activity.System, activity.Updated, activity.Via, activity.Ts, activity.Tsoff)
	return err
}

// updateActivityTsoff updates the tsoff timestamp for an activity record.
func updateActivityTsoff(ctx context.Context, db *sql.DB, id string, tsoff int64) error {
	_, err := db.ExecContext(ctx, "UPDATE activity SET tsoff = ? WHERE id = ?", tsoff, id)
	return err
}
