// Package main implements an activity monitor for a digital voice system.
// It tails a log file and records on/off information in a SQLite database,
// broadcasting events over SSE and optionally MQTT.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/eclipse/paho.golang/autopaho"
	"github.com/eclipse/paho.golang/paho"
	"github.com/nxadm/tail"
	"golang.org/x/sync/errgroup"
	_ "modernc.org/sqlite"
)

// Config holds the application configuration loaded from a TOML file.
type Config struct {
	DBPath   string     `toml:"db_path"`
	LogFile  string     `toml:"log_file"`
	HTTPAddr string     `toml:"http_addr"`
	Timezone string     `toml:"timezone"`
	System   string     `toml:"system"`
	MQTT     MQTTConfig `toml:"mqtt"`
}

// MQTTConfig holds MQTT broker connection settings.
type MQTTConfig struct {
	Enabled     bool   `toml:"enabled"`
	Broker      string `toml:"broker"`
	ClientID    string `toml:"client_id"`
	TopicPrefix string `toml:"topic_prefix"`
}

// Record represents a single activity entry.
type Record struct {
	ID     int64  `json:"id"`
	TS     int64  `json:"ts"`
	TSoff  int64  `json:"tsoff"`
	System string `json:"system"`
	Module string `json:"module"`
	Call   string `json:"call"`
	Via    string `json:"via"`
}

// ActivityEvent is the payload broadcast over SSE and MQTT.
// The nested Record matches the field names the dashboard JS already expects.
type ActivityEvent struct {
	Action string `json:"action"` // "on" or "off"
	Record Record `json:"record"`
}

// Broker manages SSE subscriber channels.
type Broker struct {
	mu          sync.Mutex
	subscribers map[chan []byte]struct{}
}

func newBroker() *Broker {
	return &Broker{subscribers: make(map[chan []byte]struct{})}
}

func (b *Broker) subscribe() chan []byte {
	ch := make(chan []byte, 8)
	b.mu.Lock()
	b.subscribers[ch] = struct{}{}
	b.mu.Unlock()
	return ch
}

func (b *Broker) unsubscribe(ch chan []byte) {
	b.mu.Lock()
	delete(b.subscribers, ch)
	b.mu.Unlock()
}

func (b *Broker) publish(data []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subscribers {
		select {
		case ch <- data:
		default: // drop if subscriber is slow
		}
	}
}

// App holds the application dependencies.
type App struct {
	config Config
	db     *sql.DB
	broker *Broker
	mqtt   *autopaho.ConnectionManager // nil if MQTT is disabled
}

// initDB opens the SQLite database, verifies connectivity, and creates the
// activity table if it does not already exist.
func initDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS activity (
		id     INTEGER PRIMARY KEY,
		ts     INTEGER NOT NULL,
		tsoff  INTEGER NOT NULL DEFAULT 0,
		system TEXT NOT NULL,
		module TEXT NOT NULL,
		call   TEXT NOT NULL,
		via    TEXT NOT NULL DEFAULT ''
	)`)
	if err != nil {
		return nil, err
	}
	_, err = db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_activity_ts_tsoff  ON activity (ts, tsoff);
	`)
	return db, err
}

// initMQTT starts an autopaho connection manager, or returns nil if MQTT is
// disabled. autopaho handles reconnection automatically; the connection is
// torn down when ctx is cancelled.
func initMQTT(ctx context.Context, cfg MQTTConfig) *autopaho.ConnectionManager {
	if !cfg.Enabled {
		return nil
	}
	u, err := url.Parse(cfg.Broker)
	if err != nil {
		slog.Error("Invalid MQTT broker URL", "broker", cfg.Broker, "error", err)
		return nil
	}
	cm, err := autopaho.NewConnection(ctx, autopaho.ClientConfig{
		ServerUrls:        []*url.URL{u},
		KeepAlive:         30,
		ConnectRetryDelay: 10 * time.Second,
		OnConnectionUp: func(cm *autopaho.ConnectionManager, connAck *paho.Connack) {
			slog.Info("MQTT connected", "broker", cfg.Broker)
		},
		ClientConfig: paho.ClientConfig{
			ClientID: cfg.ClientID,
		},
	})
	if err != nil {
		slog.Error("MQTT setup failed", "error", err)
		return nil
	}
	return cm
}

// getLastTime returns the timestamp of the most recent activity record.
func (app *App) getLastTime() (int64, error) {
	var ts int64
	err := app.db.QueryRow(
		`SELECT COALESCE(MAX(ts), 0) FROM activity WHERE system = ?`,
		app.config.System,
	).Scan(&ts)
	return ts, err
}

// broadcast serialises an ActivityEvent and sends it to all SSE subscribers
// and to MQTT if enabled. On-air events are published with retain=true so new
// subscribers learn current state; off-air events clear the retained message.
func (app *App) broadcast(evt ActivityEvent) {
	data, err := json.Marshal(evt)
	if err != nil {
		slog.Error("Failed to marshal event", "error", err)
		return
	}
	app.broker.publish(data)

	if app.mqtt != nil {
		topic := fmt.Sprintf("%s/%s", app.config.MQTT.TopicPrefix, evt.Record.Module)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		switch evt.Action {
		case "on":
			if _, err := app.mqtt.Publish(ctx, &paho.Publish{
				QoS: 0, Topic: topic, Retain: true, Payload: data,
			}); err != nil {
				slog.Warn("MQTT publish failed", "error", err)
			}
		case "off":
			if _, err := app.mqtt.Publish(ctx, &paho.Publish{
				QoS: 0, Topic: topic, Retain: false, Payload: data,
			}); err != nil {
				slog.Warn("MQTT publish failed", "error", err)
			}
			// Clear the retained on-air message so late subscribers see correct state.
			if _, err := app.mqtt.Publish(ctx, &paho.Publish{
				QoS: 0, Topic: topic, Retain: true, Payload: []byte{},
			}); err != nil {
				slog.Warn("MQTT retain clear failed", "error", err)
			}
		}
	}
}

var (
	reOpening = regexp.MustCompile(`Opening stream on module (?P<module>[A-Z]) for client (?P<client>[^\s]+)\s+(?P<clientmod>.) with sid \d{1,} by user (?P<user>.*)`)
	reClosing = regexp.MustCompile(`Closing stream of module ([A-Z])`)
)

// doTail tails the system log file and processes xlxd activity entries.
// It returns when ctx is cancelled or a fatal error occurs.
func (app *App) doTail(ctx context.Context) error {
	onair := make(map[string]Record) // module -> in-progress record

	lastTime, err := app.getLastTime()
	if err != nil {
		return fmt.Errorf("get last activity time: %w", err)
	}
	slog.Info("Retrieved last activity time", "timestamp", lastTime)

	tzLocation, err := time.LoadLocation(app.config.Timezone)
	if err != nil {
		return fmt.Errorf("load timezone %s: %w", app.config.Timezone, err)
	}

	t, err := tail.TailFile(app.config.LogFile, tail.Config{Follow: true, ReOpen: true})
	if err != nil {
		return fmt.Errorf("tail %s: %w", app.config.LogFile, err)
	}
	go func() {
		<-ctx.Done()
		t.Stop()
	}()
	defer t.Cleanup()

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
			ts = time.Now()
			slog.Error("Unable to parse time", "input", parts[0], "error", err)
		}
		uTs := ts.UnixMilli()
		if uTs <= lastTime {
			continue
		}

		slog.Debug("Processing log line", "content", line.Text)

		if groups := reOpening.FindStringSubmatch(line.Text); len(groups) == 5 {
			via := groups[2]
			if groups[3] != " " {
				via = via + "-" + groups[3]
			}
			rec := Record{
				TS:     uTs,
				TSoff:  0,
				System: app.config.System,
				Module: groups[1],
				Call:   strings.Split(groups[4], " ")[0],
				Via:    via,
			}
			result, err := app.db.Exec(
				`INSERT INTO activity (ts, tsoff, system, module, call, via) VALUES (?, 0, ?, ?, ?, ?)`,
				rec.TS, rec.System, rec.Module, rec.Call, rec.Via,
			)
			if err != nil {
				return fmt.Errorf("save record: %w", err)
			}
			rec.ID, err = result.LastInsertId()
			if err != nil {
				return fmt.Errorf("get insert id: %w", err)
			}
			onair[rec.Module] = rec
			slog.Info("+++ on  +++", "call", rec.Call, "module", rec.Module, "timestamp", uTs, "recordId", rec.ID)
			app.broadcast(ActivityEvent{Action: "on", Record: rec})
		}

		if groups := reClosing.FindStringSubmatch(line.Text); len(groups) == 2 {
			module := parts[7]
			rec, ok := onair[module]
			slog.Info("--- off ---", "module", module, "recordId", rec.ID, "timestamp", uTs)
			if ok {
				if _, err := app.db.Exec(
					`UPDATE activity SET tsoff = ? WHERE id = ?`, uTs, rec.ID,
				); err != nil {
					return fmt.Errorf("update record: %w", err)
				}
				rec.TSoff = uTs
				app.broadcast(ActivityEvent{Action: "off", Record: rec})
				delete(onair, module)
			} else {
				slog.Warn("Disconnect without connect record", "module", module)
			}
		}
	}

	return t.Err()
}

// handleEvents serves a Server-Sent Events stream of activity events.
func (app *App) handleEvents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	ch := app.broker.subscribe()
	defer app.broker.unsubscribe(ch)

	for {
		select {
		case data := <-ch:
			fmt.Fprintf(w, "data: %s\n\n", data)
			w.(http.Flusher).Flush()
		case <-r.Context().Done():
			return
		}
	}
}

// staleCutoff is how far back the recent endpoint looks, matching the dashboard JS.
const staleCutoff = 15 * 60 * 1000 // milliseconds

// handleRecent returns recent activity records as a JSON array.
func (app *App) handleRecent(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	cutoff := time.Now().UnixMilli() - staleCutoff
	rows, err := app.db.QueryContext(r.Context(),
		`SELECT id, ts, tsoff, system, module, call, via FROM activity
		 WHERE system = ? AND ts >= ? ORDER BY ts DESC`,
		app.config.System, cutoff,
	)
	if err != nil {
		slog.Error("Recent query failed", "error", err)
		http.Error(w, "query failed", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	records := []Record{}
	for rows.Next() {
		var rec Record
		if err := rows.Scan(&rec.ID, &rec.TS, &rec.TSoff, &rec.System, &rec.Module, &rec.Call, &rec.Via); err != nil {
			slog.Error("Row scan failed", "error", err)
			http.Error(w, "scan failed", http.StatusInternalServerError)
			return
		}
		records = append(records, rec)
	}
	if err := rows.Err(); err != nil {
		slog.Error("Row iteration failed", "error", err)
		http.Error(w, "query failed", http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(records)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintln(w, `{"status":"ok"}`)
}

func main() {
	configPath := flag.String("config", "activity.toml", "path to TOML config file")
	flag.Parse()

	var logLevel slog.LevelVar
	logLevel.Set(slog.LevelInfo)
	if envLevel := os.Getenv("LOG_LEVEL"); envLevel != "" {
		if err := logLevel.UnmarshalText([]byte(envLevel)); err != nil {
			fmt.Printf("Invalid LOG_LEVEL: %s, using INFO\n", envLevel)
		}
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: &logLevel})))

	var cfg Config
	if _, err := toml.DecodeFile(*configPath, &cfg); err != nil {
		slog.Error("Failed to load config", "path", *configPath, "error", err)
		os.Exit(1)
	}
	slog.Info("Activity monitor starting", "config", *configPath)

	db, err := initDB(cfg.DBPath)
	if err != nil {
		slog.Error("Failed to open database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// errgroup context is cancelled when any goroutine returns an error,
	// triggering shutdown of the remaining goroutines
	g, ctx := errgroup.WithContext(ctx)

	app := &App{
		config: cfg,
		db:     db,
		broker: newBroker(),
		mqtt:   initMQTT(ctx, cfg.MQTT),
	}

	g.Go(func() error { return app.doTail(ctx) })

	mux := http.NewServeMux()
	mux.HandleFunc("/api/activity/events", app.handleEvents)
	mux.HandleFunc("/api/activity/recent", app.handleRecent)
	mux.HandleFunc("/health", handleHealth)
	srv := &http.Server{Addr: cfg.HTTPAddr, Handler: mux}

	g.Go(func() error {
		slog.Info("HTTP server listening", "addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	})
	g.Go(func() error {
		<-ctx.Done()
		slog.Info("Shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	})

	if err := g.Wait(); err != nil {
		slog.Error("Exiting with error", "error", err)
		os.Exit(1)
	}
	slog.Info("Shutdown complete")
}

// vim:noet:ts=4
