// Package main provides functionalities for database interactions.
package main

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open in-memory database: %v", err)
	}

	_, err = db.Exec(`
        CREATE TABLE activity (
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
    `)
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	return db
}

func TestGetLastTime(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()

	t.Run("empty database", func(t *testing.T) {
		lastTime, err := getLastTime(ctx, db, "299")
		if err != nil {
			t.Fatalf("Expected no error, but got %v", err)
		}
		if lastTime != 0 {
			t.Fatalf("Expected last time to be 0, but got %v", lastTime)
		}
	})

	t.Run("with records", func(t *testing.T) {
		ts := float64(time.Now().UnixMilli())
		_, err := db.Exec("INSERT INTO activity (id, ts, system) VALUES (?, ?, ?)", "1", ts, "299")
		if err != nil {
			t.Fatalf("Failed to insert record: %v", err)
		}

		lastTime, err := getLastTime(ctx, db, "299")
		if err != nil {
			t.Fatalf("Failed to get last time: %v", err)
		}

		if lastTime != int64(ts) {
			t.Fatalf("Expected last time to be %v, but got %v", ts, lastTime)
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := getLastTime(ctx, db, "299")
		if err == nil {
			t.Fatal("Expected an error, but got none")
		}
	})
}

func TestSaveActivity(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()

	activity := Activity{
		ID:      "1",
		Call:    "TEST",
		Created: time.Now(),
		Module:  "A",
		System:  "299",
		Updated: time.Now(),
		Via:     "TEST",
		Ts:      float64(time.Now().UnixMilli()),
		Tsoff:   0,
	}

	err := saveActivity(ctx, db, activity)
	if err != nil {
		t.Fatalf("Failed to save activity: %v", err)
	}

	var id string
	err = db.QueryRow("SELECT id FROM activity WHERE id = '1'").Scan(&id)
	if err != nil {
		t.Fatalf("Failed to query activity: %v", err)
	}
	if id != "1" {
		t.Fatal("Activity not saved correctly")
	}
}

func TestUpdateActivityTsoff(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()

	_, err := db.Exec("INSERT INTO activity (id) VALUES (?)", "1")
	if err != nil {
		t.Fatalf("Failed to insert record: %v", err)
	}

	tsoff := time.Now().UnixMilli()
	err = updateActivityTsoff(ctx, db, "1", tsoff)
	if err != nil {
		t.Fatalf("Failed to update tsoff: %v", err)
	}

	var updatedTsoff int64
	err = db.QueryRow("SELECT tsoff FROM activity WHERE id = '1'").Scan(&updatedTsoff)
	if err != nil {
		t.Fatalf("Failed to query tsoff: %v", err)
	}

	if updatedTsoff != tsoff {
		t.Fatalf("Expected tsoff to be %v, but got %v", tsoff, updatedTsoff)
	}
}
