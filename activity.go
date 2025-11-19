// Package main implements an activity monitor for digital voice systems.
// It tails a log file and records on/off information in a SQLite database.
// The program monitors XLX digital voice system logs to track when users connect
// and disconnect from modules, recording the activity with timestamps.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/nxadm/tail"
)

// Regex patterns for parsing log entries
var (
	reOpening = regexp.MustCompile(`Opening stream on module (?P<module>[A-Z]) for client (?P<client>[^\s]+)\s+(?P<clientmod>.) with sid \d{1,} by user (?P<user>.*)`)
	reClosing = regexp.MustCompile(`Closing stream of module ([A-Z])`)
)

// doTail tails the system log file and processes entries related to XLX activity.
func doTail(ctx context.Context, wg *sync.WaitGroup, db *sql.DB, config *Config, b *Broadcaster) {
	defer wg.Done()
	onair := make(map[string]Activity) // map of module to last activity

	t, err := tail.TailFile(
		config.LogPath, tail.Config{Follow: true, ReOpen: true})
	if err != nil {
		slog.Error("Failed to tail file", "error", err)
		return
	}

	lastTime, err := getLastTime(ctx, db, config.SystemName)
	if err != nil {
		slog.Error("Failed to get last activity time", "error", err)
		return
	}
	slog.Info("Retrieved last activity time", "timestamp", lastTime)

	tzLocation, err := time.LoadLocation(config.Timezone)
	if err != nil {
		slog.Error("Failed to load timezone", "error", err)
		return
	}

	for {
		select {
		case <-ctx.Done():
			slog.Info("Stopping tailing")
			t.Stop()
			return
		case line := <-t.Lines:
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
				via := groups[2]
				if groups[3] != " " {
					via = via + "-" + groups[3]
				}
				activity := Activity{
					ID:      uuid.New().String(),
					Ts:      float64(uTs),
					Tsoff:   0,
					System:  config.SystemName,
					Module:  groups[1],
					Call:    strings.Split(groups[4], " ")[0],
					Via:     via,
					Created: time.Now(),
					Updated: time.Now(),
				}
				if err := saveActivity(ctx, db, activity); err != nil {
					slog.Error("Failed to save record", "error", err)
					continue
				}
				// save the Id of the onair record
				onair[groups[1]] = activity
				b.Submit(activity)
				slog.Info("+++ on  +++",
					"call", activity.Call,
					"module", activity.Module,
					"timestamp", uTs,
					"recordId", activity.ID)
			}
			groups = reClosing.FindStringSubmatch(line.Text)
			if len(groups) == 2 {
				module := parts[7]
				activity, ok := onair[module]
				slog.Info("--- off ---", "module", module, "recordId", activity.ID, "timestamp", uTs)
				if ok {
					if err := updateActivityTsoff(ctx, db, activity.ID, uTs); err != nil {
						slog.Error("Failed to update record", "error", err)
						continue
					}
					activity.Tsoff = uTs
					b.Submit(activity)
				} else {
					slog.Warn("Disconnect without connect record", "module", module)
				}
			}
		}
	}
}

// main is the entry point for the activity monitor application.
func main() {
	config := LoadConfig()

	var logLevel slog.LevelVar
	logLevel.Set(slog.LevelDebug) // Default level
	if envLevel := os.Getenv("LOG_LEVEL"); envLevel != "" {
		if err := logLevel.UnmarshalText([]byte(envLevel)); err != nil {
			fmt.Printf("Invalid LOG_LEVEL: %s, using INFO\n", envLevel)
		}
	}

	logHandler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: &logLevel,
	})
	slog.SetDefault(slog.New(logHandler))

	slog.Info("Activity monitor starting",
		"args", os.Args,
		"system_name", config.SystemName,
		"timezone", config.Timezone,
		"db_path", config.DBPath,
		"sse_addr", config.SSEAddr,
		"log_path", config.LogPath)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	db, err := initDB(config.DBPath)
	if err != nil {
		slog.Error("Failed to initialize database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	broadcaster := NewBroadcaster()

	var wg sync.WaitGroup
	wg.Add(3) // doTail, startSSE, and broadcaster

	go broadcaster.Run(ctx, &wg)
	go doTail(ctx, &wg, db, config, broadcaster)
	go startSSE(ctx, &wg, config.SSEAddr, broadcaster)

	<-ctx.Done()
	slog.Info("Shutting down...")
	wg.Wait()
	slog.Info("Shutdown complete")
}

// vim:noet:ts=4
