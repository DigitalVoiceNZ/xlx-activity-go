// Package main provides tests for the activity monitor.
package main

import (
	"regexp"
	"testing"
)

func TestLogParsing(t *testing.T) {
	testCases := []struct {
		name    string
		logLine string
		regex   *regexp.Regexp
		matches bool
	}{
		{
			name:    "opening stream",
			logLine: "Opening stream on module A for client DVSWITCH   . with sid 2001 by user ZL4FOX",
			regex:   reOpening,
			matches: true,
		},
		{
			name:    "closing stream",
			logLine: "Closing stream of module A",
			regex:   reClosing,
			matches: true,
		},
		{
			name:    "unrelated log line",
			logLine: "This is not a relevant log line",
			regex:   reOpening,
			matches: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			matches := tc.regex.FindStringSubmatch(tc.logLine)
			if (len(matches) > 0) != tc.matches {
				t.Errorf("Expected matches to be %v, but got %v", tc.matches, len(matches) > 0)
			}
		})
	}
}
