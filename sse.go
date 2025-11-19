// Package main provides functionalities for handling Server-Sent Events (SSE).
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
)

// sseHandler creates an HTTP handler for an SSE stream that registers
// itself with a broadcaster.
func sseHandler(b *Broadcaster) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
			return
		}

		// Create a channel for this specific client.
		clientChan := make(chan Activity, 10) // Buffer to absorb some burstiness.
		b.Register(clientChan)
		defer b.Unregister(clientChan)

		slog.Info("Client connected to SSE endpoint")
		fmt.Fprintf(w, "data: %s\n\n", "Welcome to the activity stream!")
		flusher.Flush()

		ctx := r.Context()
		for {
			select {
			case msg := <-clientChan:
				data, err := json.Marshal(msg)
				if err != nil {
					slog.Error("Failed to marshal activity for SSE", "error", err)
					continue
				}
				fmt.Fprintf(w, "data: %s\n\n", data)
				flusher.Flush()
			case <-ctx.Done():
				slog.Info("Client disconnected from SSE endpoint")
				return
			}
		}
	}
}

// startSSE starts the HTTP server for the SSE endpoint.
func startSSE(ctx context.Context, wg *sync.WaitGroup, addr string, b *Broadcaster) {
	defer wg.Done()

	mux := http.NewServeMux()
	mux.HandleFunc("/sse", sseHandler(b))

	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	go func() {
		slog.Info("Starting SSE server", "address", addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Failed to start SSE server", "error", err)
		}
	}()

	<-ctx.Done()
	slog.Info("Shutting down SSE server")
	if err := server.Shutdown(context.Background()); err != nil {
		slog.Error("SSE server shutdown failed", "error", err)
	}
}
