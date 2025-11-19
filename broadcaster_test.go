// Package main provides tests for the broadcaster.
package main

import (
	"context"
	"sync"
	"testing"
	"time"
)

// TestBroadcaster provides unit tests for the Broadcaster struct and its methods.
func TestBroadcaster(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	b := NewBroadcaster()

	wg.Add(1)
	go b.Run(ctx, &wg)

	t.Run("register and broadcast", func(t *testing.T) {
		client1 := make(chan Activity, 1)
		b.Register(client1)

		testActivity := Activity{ID: "test1"}
		b.Submit(testActivity)

		select {
		case received := <-client1:
			if received.ID != testActivity.ID {
				t.Errorf("Expected activity ID %s, but got %s", testActivity.ID, received.ID)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatal("Test timed out: client did not receive message")
		}
	})

	t.Run("unregister client", func(t *testing.T) {
		client2 := make(chan Activity, 1)
		b.Register(client2)

		// Ensure client is registered before unregistering
		time.Sleep(50 * time.Millisecond)

		b.Unregister(client2)

		// Wait for the broadcaster to close the channel, confirming unregistration.
		select {
		case _, ok := <-client2:
			if ok {
				t.Fatal("Channel should be closed, but received a message")
			}
			// Channel is closed, which is what we expect.
		case <-time.After(100 * time.Millisecond):
			t.Fatal("Test timed out: broadcaster did not close client channel")
		}

		// Now that we've confirmed unregistration, submitting should have no effect on client2.
		b.Submit(Activity{ID: "test2"})

		// Double-check that no message was sent after the fact.
		select {
		case msg, ok := <-client2:
			if ok {
				t.Fatalf("Unregistered client received a message after confirmation: %+v", msg)
			}
		default:
			// Test passed, the channel remains closed and empty.
		}
	})

	t.Run("shutdown on context cancel", func(t *testing.T) {
		// The main context is canceled by the defer at the top of the test function.
		// We just need to wait for the broadcaster's goroutine to finish.
		shutdownCtx, shutdownCancel := context.WithCancel(context.Background())
		var shutdownWg sync.WaitGroup
		
		testBroadcaster := NewBroadcaster()
		shutdownWg.Add(1)
		go testBroadcaster.Run(shutdownCtx, &shutdownWg)

		// Cancel the context to trigger shutdown
		shutdownCancel()

		done := make(chan struct{})
		go func() {
			shutdownWg.Wait()
			close(done)
		}()

		select {
		case <-done:
			// Test passed
		case <-time.After(1 * time.Second):
			t.Fatal("Broadcaster did not shut down within the time limit")
		}
	})
}
