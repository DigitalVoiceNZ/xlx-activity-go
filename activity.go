// Package main implements an activity monitor for digital voice systems.
// It tails a log file and records on/off information in a PocketBase database.
// The program monitors XLX digital voice system logs to track when users connect
// and disconnect from modules, recording the activity with timestamps.
package main

import (
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/nxadm/tail"
	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/daos"
	"github.com/pocketbase/pocketbase/models"
)

// getLastTime retrieves the timestamp of the most recent activity record for system "299".
// Parameters:
//   - dao: Data Access Object for database operations
//
// Returns:
//   - int64: The timestamp of the most recent record in milliseconds since epoch
//   - error: Any error encountered during the database query
func getLastTime(dao *daos.Dao) (int64, error) {
	collection, err := dao.FindCollectionByNameOrId("activity")
	if err != nil {
		return 0, err
	}

	query := dao.RecordQuery(collection).
		AndWhere(dbx.HashExp{"system": "299"}).
		OrderBy("ts DESC").
		Limit(1)

	rows := []dbx.NullStringMap{}
	if err := query.All(&rows); err != nil {
		return 0, err
	}

	return int64(models.NewRecordsFromNullStringMaps(collection, rows)[0].GetFloat("ts")), nil
}

// Regex patterns for parsing log entries
var (
	reOpening = regexp.MustCompile(`Opening stream on module (?P<module>[A-Z]) for client (?P<client>[^\s]+)\s+(?P<clientmod>.) with sid \d{1,} by user (?P<user>.*)`)
	reClosing = regexp.MustCompile(`Closing stream of module ([A-Z])`)
)

// doTail tails the system log file and processes entries related to XLX activity.
// It records connection and disconnection events in the PocketBase database.
// Parameters:
//   - a: A pointer to the PocketBase instance for database operations
func doTail(a *pocketbase.PocketBase) {
	onair := make(map[string]string) // map of module to last record id

	time.Sleep(1 * time.Second)
	t, err := tail.TailFile(
		"/var/log/syslog", tail.Config{Follow: true, ReOpen: true})
	if err != nil {
		panic(err)
	}

	collection, err := a.Dao().FindCollectionByNameOrId("activity")
	if err != nil {
		slog.Error("Failed to find collection", "error", err)
		os.Exit(1)
	}

	lastTime, err := getLastTime(a.Dao())
	if err != nil {
		slog.Error("Failed to find collection", "error", err)
		os.Exit(1)
	}
	slog.Info("Retrieved last activity time", "timestamp", lastTime)

	time.Sleep(4 * time.Second)
	// Print the text of each received line
	tzLocation, err := time.LoadLocation("Pacific/Auckland")
	if err != nil {
		slog.Error("Failed to find collection", "error", err)
		os.Exit(1)
	}
	for line := range t.Lines {
		parts := strings.Split(line.Text, " ")
		if len(parts) < 3 || parts[2] != "xlxd:" {
			continue
		}
		if strings.Contains(line.Text, "Sending connect packet to XLX peer") {
			continue
		}
		ts, err := time.ParseInLocation(time.RFC3339Nano, parts[0], tzLocation)
		if err != nil {
			// tail sometimes leaves a truncated date
			ts = time.Now() // or maybe last parsed time plus inc
			slog.Error("Unable to parse time", "input", parts[0], "error", err)
		}
		uTs := ts.UnixMilli()
		if uTs <= lastTime {
			continue
		}
		slog.Debug("Processing log line", "content", line.Text)
		groups := reOpening.FindStringSubmatch(line.Text)
		if len(groups) == 5 {
			record := models.NewRecord(collection)
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
			if err := a.Dao().SaveRecord(record); err != nil {
				slog.Error("Failed to save record", "error", err)
				os.Exit(1)
			}
			// save the Id of the onair record
			onair[groups[1]] = record.Id
			slog.Info("+++ on  +++",
				"call", strings.Split(groups[4], " ")[0],
				"module", groups[1],
				"timestamp", uTs,
				"recordId", record.Id)
		}
		groups = reClosing.FindStringSubmatch(line.Text)
		if len(groups) == 2 {
			module := parts[7]
			id, ok := onair[module]
			slog.Info("--- off ---", "module", module, "recordId", id, "timestamp", uTs)
			if ok {
				record, err := a.Dao().FindRecordById("activity", id)
				if err != nil {
					slog.Error("Failed to find record", "error", err)
					os.Exit(1)
				}
				record.Set("tsoff", uTs)
				if err := a.Dao().SaveRecord(record); err != nil {
					slog.Error("Failed to save record", "error", err)
					os.Exit(1)
				}
			} else {
				slog.Warn("Disconnect without connect record", "module", module)
			}
		}
	}

	slog.Debug("about to cleanup tailing")
	t.Cleanup()
	slog.Debug("clean")
}

// main initializes and starts the activity monitoring application.
// It bootstraps the PocketBase instance and starts the log monitoring in a separate goroutine.
func main() {
	// Configure structured logger with environment variable support
	// Parse log level from LOG_LEVEL env var (default to INFO if not set or invalid)
	var logLevel slog.LevelVar
	logLevel.Set(slog.LevelDebug) // Default level

	// Parse level from environment (empty string if not set)
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
	app := pocketbase.New()

	if err := app.Bootstrap(); err != nil {
		slog.Error("Failed to bootstrap application", "error", err)
		os.Exit(1)
	}

	go doTail(app)

	if err := app.Start(); err != nil {
		slog.Error("Failed to start application", "error", err)
		os.Exit(1)
	}

}

// vim:noet:ts=4
