// Package main provides functionalities for database interactions.
package main

import (
	"context"
	"database/sql"
	"log/slog"
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

// initDB initializes the SQLite database and creates the activity table if it doesn't exist.
func initDB(dbPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}

	// Create the activity table if it doesn't exist.
	_, err = db.Exec(`
        CREATE TABLE IF NOT EXISTS activity (
            call TEXT DEFAULT '',
            created TEXT DEFAULT '' NOT NULL,
            id TEXT PRIMARY KEY NOT NULL,
            module TEXT DEFAULT '',
            system TEXT DEFAULT '',
            updated TEXT DEFAULT '' NOT NULL,
            via TEXT DEFAULT '',
            ts REAL DEFAULT 0,
            tsoff NUMERIC DEFAULT 0
        );
        CREATE INDEX IF NOT EXISTS _sjde62mkgzabz32_created_idx ON activity (created);
        CREATE INDEX IF NOT EXISTS callmodts ON activity (
            call,
            module,
            tsoff
        );
    `)
	if err != nil {
		return nil, err
	}

	slog.Info("Database initialized successfully")
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
