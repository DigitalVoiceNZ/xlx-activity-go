// Package main implements an activity monitor for digital voice systems.
// It reads journald logs using journalctl and records connection/disconnection
// information in a PocketBase database.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

// getLastTime retrieves the timestamp of the most recent activity record for system "299".
// Parameters:
//   - app: PocketBase core.App instance
//
// Returns:
//   - int64: The timestamp of the most recent record in milliseconds since epoch
//   - error: Any error encountered during the database query
func getLastTime(app core.App) (int64, error) {
	var records []*core.Record
	err := app.RecordQuery("activity").
		AndWhere(dbx.HashExp{"system": "299"}).
		OrderBy("ts DESC").
		Limit(1).
		All(&records)
	if err != nil {
		return 0, err
	}

	if len(records) == 0 {
		return 0, nil
	}

	// PocketBase stores numbers as REAL (float64) in SQLite
	return int64(records[0].GetFloat("ts")), nil
}

// Regex patterns for parsing log entries
var (
	reOpening = regexp.MustCompile(`Opening stream on module (?P<module>[A-Z]) for client (?P<client>[^\s]+)\s+(?P<clientmod>.) with sid \d{1,} by user (?P<user>.*)`)
	reClosing = regexp.MustCompile(`Closing stream of module ([A-Z])`)
)

// JournalEntry represents the selected fields of a journald JSON log entry.
type JournalEntry struct {
	Cursor            string `json:"__CURSOR"`
	Message           string `json:"MESSAGE"`
	RealtimeTimestamp string `json:"__REALTIME_TIMESTAMP"`
	SyslogIdentifier  string `json:"SYSLOG_IDENTIFIER"`
}

// readCursor reads the journald cursor from the specified file path.
func readCursor(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// writeCursor writes the journald cursor to the specified file path.
func writeCursor(path string, cursor string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(cursor), 0644)
}

// startJournalTail starts journalctl to tail xlxd logs and processes entries.
func startJournalTail(ctx context.Context, app core.App) {
	collection, err := app.FindCollectionByNameOrId("activity")
	if err != nil {
		slog.Error("Failed to find collection 'activity'", "error", err)
		return
	}

	cursorPath := filepath.Join(app.DataDir(), "journal_cursor")
	cursorVal, err := readCursor(cursorPath)
	if err != nil {
		slog.Error("Failed to read cursor file", "path", cursorPath, "error", err)
	}

	lastTime, err := getLastTime(app)
	if err != nil {
		slog.Error("Failed to retrieve last activity time", "error", err)
		return
	}
	slog.Info("Retrieved last activity time", "timestamp", lastTime)

	// Prepare command arguments for journalctl
	cmdArgs := []string{"--follow", "-o", "json", "SYSLOG_IDENTIFIER=xlxd"}

	if cursorVal != "" {
		slog.Info("Starting journalctl using cursor", "cursor", cursorVal)
		cmdArgs = append(cmdArgs, fmt.Sprintf("--after-cursor=%s", cursorVal))
	} else if lastTime > 0 {
		sinceTime := time.UnixMilli(lastTime).UTC().Format("2006-01-02 15:04:05")
		slog.Info("Starting journalctl since database lastTime", "time", sinceTime)
		cmdArgs = append(cmdArgs, fmt.Sprintf("--since=%s", sinceTime))
	} else {
		slog.Info("Starting journalctl from the beginning of today")
		cmdArgs = append(cmdArgs, "--since=today")
	}

	cmd := exec.CommandContext(ctx, "journalctl", cmdArgs...)
	// Use modern Go 1.20+ Cancel field to send SIGINT for graceful shutdown of journalctl
	cmd.Cancel = func() error {
		if cmd.Process != nil {
			return cmd.Process.Signal(os.Interrupt)
		}
		return nil
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		slog.Error("Failed to create stdout pipe for journalctl", "error", err)
		return
	}

	if err := cmd.Start(); err != nil {
		slog.Error("Failed to start journalctl", "error", err)
		return
	}

	onair := make(map[string]string) // map of module to last record id

	// Populate onair map with any currently active connections from database
	var activeRecords []*core.Record
	err = app.RecordQuery("activity").
		AndWhere(dbx.HashExp{"system": "299", "tsoff": 0}).
		All(&activeRecords)
	if err != nil {
		slog.Error("Failed to retrieve active records from DB", "error", err)
	} else {
		for _, rec := range activeRecords {
			module := rec.GetString("module")
			if module != "" {
				onair[module] = rec.Id
				slog.Info("Restored active session from DB on start", "module", module, "recordId", rec.Id)
			}
		}
	}

	scanner := bufio.NewScanner(stdout)
	var lastCursor string

	defer func() {
		// Wait for command exit to clean up resource/zombie processes
		_ = cmd.Wait()
		if lastCursor != "" {
			if err := writeCursor(cursorPath, lastCursor); err != nil {
				slog.Error("Failed to save final cursor", "error", err)
			} else {
				slog.Info("Saved final cursor on shutdown", "cursor", lastCursor)
			}
		}
	}()

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var entry JournalEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			slog.Error("Failed to parse journal line JSON", "error", err, "line", string(line))
			continue
		}

		if entry.Cursor != "" {
			lastCursor = entry.Cursor
		}

		if strings.Contains(entry.Message, "Sending connect packet to XLX peer") {
			continue
		}

		// Parse the timestamp of the entry in milliseconds
		var uTs int64
		if entry.RealtimeTimestamp != "" {
			usec, err := strconv.ParseInt(entry.RealtimeTimestamp, 10, 64)
			if err != nil {
				slog.Error("Failed to parse realtime timestamp", "value", entry.RealtimeTimestamp, "error", err)
				uTs = time.Now().UnixMilli()
			} else {
				uTs = usec / 1000
			}
		} else {
			uTs = time.Now().UnixMilli()
		}

		// Safety check: skip processed logs if not using cursor and timestamp is older than lastTime from DB
		if cursorVal == "" && lastTime > 0 && uTs <= lastTime {
			continue
		}

		slog.Debug("Processing log line", "content", entry.Message)

		// Parse connection events
		groups := reOpening.FindStringSubmatch(entry.Message)
		if len(groups) == 5 {
			record := core.NewRecord(collection)
			via := groups[2]
			if groups[3] != " " {
				via = via + "-" + groups[3]
			}
			record.Set("ts", uTs)
			record.Set("tsoff", 0)
			record.Set("system", "299")
			record.Set("module", groups[1])
			record.Set("call", strings.Split(groups[4], " ")[0])
			record.Set("via", via)

			if err := app.Save(record); err != nil {
				slog.Error("Failed to save record", "error", err)
				continue
			}

			onair[groups[1]] = record.Id
			slog.Info("+++ on  +++",
				"call", strings.Split(groups[4], " ")[0],
				"module", groups[1],
				"timestamp", uTs,
				"recordId", record.Id)

			// Persist cursor immediately upon successfully processing event
			if lastCursor != "" {
				if err := writeCursor(cursorPath, lastCursor); err != nil {
					slog.Error("Failed to save cursor", "error", err)
				}
			}
		}

		// Parse disconnection events
		groups = reClosing.FindStringSubmatch(entry.Message)
		if len(groups) == 2 {
			module := groups[1]
			id, ok := onair[module]
			slog.Info("--- off ---", "module", module, "recordId", id, "timestamp", uTs)
			if ok {
				record, err := app.FindRecordById("activity", id)
				if err != nil {
					slog.Error("Failed to find record", "id", id, "error", err)
					continue
				}
				record.Set("tsoff", uTs)
				if err := app.Save(record); err != nil {
					slog.Error("Failed to save record", "id", id, "error", err)
					continue
				}
				delete(onair, module)
			} else {
				slog.Warn("Disconnect without connect record", "module", module)
			}

			// Persist cursor immediately upon successfully processing event
			if lastCursor != "" {
				if err := writeCursor(cursorPath, lastCursor); err != nil {
					slog.Error("Failed to save cursor", "error", err)
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		// Ignore EOF / closed pipe errors during shutdown
		if ctx.Err() == nil {
			slog.Error("Scanner read error", "error", err)
		}
	}
}

func main() {
	var logLevel slog.LevelVar
	logLevel.Set(slog.LevelInfo) // Default level

	if envLevel := os.Getenv("LOG_LEVEL"); envLevel != "" {
		if err := logLevel.UnmarshalText([]byte(envLevel)); err != nil {
			fmt.Printf("Invalid LOG_LEVEL: %s, using INFO\n", envLevel)
		}
	}

	logHandler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: &logLevel,
	})
	slog.SetDefault(slog.New(logHandler))

	slog.Info("Activity monitor starting", "args", os.Args)

	// Set up cancellation context for graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	app := pocketbase.New()

	var wg sync.WaitGroup

	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		wg.Add(1)
		go func() {
			defer wg.Done()
			startJournalTail(ctx, app)
		}()
		return se.Next()
	})

	if err := app.Start(); err != nil {
		slog.Error("Failed to start application", "error", err)
		os.Exit(1)
	}

	slog.Info("Waiting for background tasks to complete...")
	wg.Wait()
	slog.Info("Application shutdown complete.")
}
