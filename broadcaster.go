// Package main provides a broadcaster for distributing events to multiple clients.
package main

import (
	"context"
	"log/slog"
	"sync"
)

// Broadcaster manages a set of clients and broadcasts messages to them.
type Broadcaster struct {
	// Channel for submitting new messages to be broadcast.
	submit chan Activity

	// Channel for registering new clients.
	register chan chan<- Activity

	// Channel for unregistering clients.
	unregister chan chan<- Activity

	// Set of all registered client channels.
	clients map[chan<- Activity]bool
}

// NewBroadcaster creates and returns a new Broadcaster.
func NewBroadcaster() *Broadcaster {
	return &Broadcaster{
		submit:     make(chan Activity, 100),
		register:   make(chan chan<- Activity),
		unregister: make(chan chan<- Activity),
		clients:    make(map[chan<- Activity]bool),
	}
}

// Run starts the broadcaster's main loop.
// It should be run in a separate goroutine.
// It listens on its channels and manages clients and messages.
func (b *Broadcaster) Run(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		select {
		case <-ctx.Done():
			slog.Info("Broadcaster shutting down")
			return
		case client := <-b.register:
			slog.Info("Broadcaster registering new client")
			b.clients[client] = true
		case client := <-b.unregister:
			slog.Info("Broadcaster unregistering client")
			if _, ok := b.clients[client]; ok {
				delete(b.clients, client)
				close(client)
			}
		case msg := <-b.submit:
			slog.Debug("Broadcaster broadcasting message to clients", "client_count", len(b.clients))
			for client := range b.clients {
				// Use a select to prevent blocking if a client's buffer is full.
				select {
				case client <- msg:
				default:
					slog.Warn("Client channel buffer full, dropping message for one client")
				}
			}
		}
	}
}

// Register adds a new client channel to the broadcaster.
func (b *Broadcaster) Register(client chan<- Activity) {
	b.register <- client
}

// Unregister removes a client channel from the broadcaster.
func (b *Broadcaster) Unregister(client chan<- Activity) {
	b.unregister <- client
}

// Submit sends a new message to the broadcaster to be sent to all clients.
func (b *Broadcaster) Submit(msg Activity) {
	b.submit <- msg
}
