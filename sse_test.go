// Package main provides functionalities for handling Server-Sent Events (SSE).
package main

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSseWithBroadcaster(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	b := NewBroadcaster()

	wg.Add(1)
	go b.Run(ctx, &wg)

	// Use httptest.NewServer to run the handler on a real server
	server := httptest.NewServer(sseHandler(b))
	defer server.Close()

	t.Run("single client receives broadcast", func(t *testing.T) {
		// Make a request to the test server
		res, err := http.Get(server.URL)
		if err != nil {
			t.Fatalf("Failed to make request to test server: %v", err)
		}
		defer res.Body.Close()

		// Give the handler a moment to register the client
		time.Sleep(50 * time.Millisecond)

		// Submit an event
		activity := Activity{ID: "1"}
		b.Submit(activity)

		// Read the event from the response body
		scanner := bufio.NewScanner(res.Body)
		found := false
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "data:") {
				data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
				if strings.Contains(data, `"ID":"1"`) {
					found = true
					break // Exit after finding the message
				}
			}
		}

		if !found {
			t.Error("Expected to find activity with ID 1, but didn't")
		}
	})

	t.Run("multiple clients receive broadcast", func(t *testing.T) {
		numClients := 3
		var wg sync.WaitGroup
		wg.Add(numClients)

		for i := 0; i < numClients; i++ {
			go func() {
				defer wg.Done()
				res, err := http.Get(server.URL)
				if err != nil {
					t.Errorf("Failed to make request for a client: %v", err)
					return
				}
				defer res.Body.Close()

				scanner := bufio.NewScanner(res.Body)
				for scanner.Scan() {
					line := scanner.Text()
					if strings.HasPrefix(line, "data:") {
						data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
						if strings.Contains(data, `"ID":"2"`) {
							return // Found the message, exit goroutine
						}
					}
				}
			}()
		}

		// Give clients time to connect
		time.Sleep(100 * time.Millisecond)

		// Submit an event to be broadcast
		activity := Activity{ID: "2"}
		b.Submit(activity)

		// Wait for all clients to find the message
		// A timeout is useful here to prevent the test from hanging indefinitely
		// if something goes wrong.
		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		select {
		case <-done:
			// All goroutines completed successfully
		case <-time.After(2 * time.Second):
			t.Fatal("Test timed out, not all clients received the message")
		}
	})
}
