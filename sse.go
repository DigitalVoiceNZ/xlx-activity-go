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

// ActivityChannel is a channel for broadcasting activity events.
var ActivityChannel = make(chan Activity)

// sseHandler handles SSE requests and streams activity events to clients.
func sseHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
		return
	}

	slog.Info("Client connected to SSE endpoint")

	// Send a welcome message
	fmt.Fprintf(w, "data: %s\n\n", "Welcome to the activity stream!")
	flusher.Flush()

	for {
		select {
		case activity := <-ActivityChannel:
			data, err := json.Marshal(activity)
			if err != nil {
				slog.Error("Failed to marshal activity", "error", err)
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		case <-r.Context().Done():
			slog.Info("Client disconnected from SSE endpoint")
			return
		}
	}
}

// startSSE starts the HTTP server for the SSE endpoint.
func startSSE(ctx context.Context, wg *sync.WaitGroup, addr string) {
	defer wg.Done()

	mux := http.NewServeMux()
	mux.HandleFunc("/sse", sseHandler)

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
