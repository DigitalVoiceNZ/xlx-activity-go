// Package main provides functionalities for handling Server-Sent Events (SSE).
package main

import (
	"bufio"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSseHandler(t *testing.T) {
	handler := http.HandlerFunc(sseHandler)

	t.Run("single client", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/sse", nil)
		rr := httptest.NewRecorder()

		go handler.ServeHTTP(rr, req)

		time.Sleep(100 * time.Millisecond)

		activity := Activity{ID: "1"}
		ActivityChannel <- activity

		time.Sleep(100 * time.Millisecond)

		scanner := bufio.NewScanner(rr.Body)
		found := false
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "data:") {
				data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
				if strings.Contains(data, `"ID":"1"`) {
					found = true
					break
				}
			}
		}
		if !found {
			t.Error("Expected to find activity with ID 1, but didn't")
		}
	})

	t.Run("multiple clients", func(t *testing.T) {
		var wg sync.WaitGroup
		numClients := 3

		for i := 0; i < numClients; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				req := httptest.NewRequest(http.MethodGet, "/sse", nil)
				rr := httptest.NewRecorder()

				go handler.ServeHTTP(rr, req)

				time.Sleep(100 * time.Millisecond)

				scanner := bufio.NewScanner(rr.Body)
				for scanner.Scan() {
					line := scanner.Text()
					if strings.HasPrefix(line, "data:") {
						data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
						if strings.Contains(data, `"ID":"2"`) {
							return
						}
					}
				}
			}()
		}

		time.Sleep(100 * time.Millisecond)
		activity := Activity{ID: "2"}
		ActivityChannel <- activity

		wg.Wait()
	})
}
