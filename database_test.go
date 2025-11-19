// Package main provides functionalities for database interactions.
package main

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
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

	// Load test data
	data, err := os.ReadFile("testdata/testdata.sql")
	if err != nil {
		t.Fatalf("Failed to read testdata/testdata.sql: %v", err)
	}
	if _, err := db.Exec(string(data)); err != nil {
		t.Fatalf("Failed to load test data: %v", err)
	}

	return db
}

func TestLoadTestData(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM activity").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to count records: %v", err)
	}
	if count != 20 {
		t.Errorf("Expected 20 records, but got %d", count)
	}
}

func TestGetLastTime(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()

	t.Run("with test data", func(t *testing.T) {
		lastTime, err := getLastTime(ctx, db, "299")
		if err != nil {
			t.Fatalf("Failed to get last time: %v", err)
		}
		if lastTime == 0 {
			t.Fatal("Expected last time to be non-zero, but got 0")
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
		ID:      "new-activity",
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
	err = db.QueryRow("SELECT id FROM activity WHERE id = 'new-activity'").Scan(&id)
	if err != nil {
		t.Fatalf("Failed to query activity: %v", err)
	}
	if id != "new-activity" {
		t.Fatal("Activity not saved correctly")
	}
}

func TestUpdateActivityTsoff(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ctx := context.Background()

	// Get an ID from the test data
	var existingID string
	err := db.QueryRow("SELECT id FROM activity LIMIT 1").Scan(&existingID)
	if err != nil {
		t.Fatalf("Failed to get an existing ID from test data: %v", err)
	}

	tsoff := time.Now().UnixMilli()
	err = updateActivityTsoff(ctx, db, existingID, tsoff)
	if err != nil {
		t.Fatalf("Failed to update tsoff: %v", err)
	}

	var updatedTsoff int64
	err = db.QueryRow("SELECT tsoff FROM activity WHERE id = ?", existingID).Scan(&updatedTsoff)
	if err != nil {
		t.Fatalf("Failed to query tsoff: %v", err)
	}

	if updatedTsoff != tsoff {
		t.Fatalf("Expected tsoff to be %v, but got %v", tsoff, updatedTsoff)
	}
}

func TestInitDB_Creation(t *testing.T) {
	// Generate a temporary file path, but ensure the file doesn't exist.
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	// Call initDB with create = true on a non-existent path
	db, err := initDB(dbPath, true)
	if err != nil {
		t.Fatalf("initDB with create=true failed: %v", err)
	}
	defer db.Close()

	// Check that the table was created
	var tableName string
	err = db.QueryRow("SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'activity'").Scan(&tableName)
	if err != nil {
		if err == sql.ErrNoRows {
			t.Fatal("Table 'activity' was not created")
		}
		t.Fatalf("Failed to query for table: %v", err)
	}

	// Check that the index was created
	var indexName string
	err = db.QueryRow("SELECT name FROM sqlite_master WHERE type = 'index' AND name = 'callmodts'").Scan(&indexName)
	if err != nil {
		if err == sql.ErrNoRows {
			t.Fatal("Index 'callmodts' was not created")
		}
		t.Fatalf("Failed to query for index: %v", err)
	}
}
