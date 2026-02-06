/*
Copyright 2024 ayoy.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package quay

import (
	"testing"
)

func TestParseSyncInterval(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
		wantErr  bool
	}{
		{name: "empty string defaults to 86400", input: "", expected: 86400},
		{name: "bare seconds", input: "86400", expected: 86400},
		{name: "seconds suffix", input: "3600s", expected: 3600},
		{name: "minutes suffix", input: "60m", expected: 3600},
		{name: "hours suffix", input: "8h", expected: 28800},
		{name: "days suffix", input: "2d", expected: 172800},
		{name: "weeks suffix", input: "1w", expected: 604800},
		{name: "one second", input: "1s", expected: 1},
		{name: "one minute", input: "1m", expected: 60},
		{name: "one day", input: "1d", expected: 86400},
		{name: "invalid suffix", input: "10x", wantErr: true},
		{name: "invalid number", input: "abcs", wantErr: true},
		{name: "invalid bare string", input: "abc", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseSyncInterval(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseSyncInterval(%q) expected error, got %d", tt.input, result)
				}
				return
			}
			if err != nil {
				t.Errorf("ParseSyncInterval(%q) unexpected error: %v", tt.input, err)
				return
			}
			if result != tt.expected {
				t.Errorf("ParseSyncInterval(%q) = %d, want %d", tt.input, result, tt.expected)
			}
		})
	}
}

func TestNewClient(t *testing.T) {
	c := NewClient("https://quay.example.com", "test-token")
	if c.baseURL != "https://quay.example.com" {
		t.Errorf("expected baseURL 'https://quay.example.com', got %q", c.baseURL)
	}
	if c.token != "test-token" {
		t.Errorf("expected token 'test-token', got %q", c.token)
	}
}

func TestNewClientTrimsTrailingSlash(t *testing.T) {
	c := NewClient("https://quay.example.com/", "test-token")
	if c.baseURL != "https://quay.example.com" {
		t.Errorf("expected baseURL 'https://quay.example.com', got %q", c.baseURL)
	}
}
