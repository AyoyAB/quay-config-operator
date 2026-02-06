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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetMirror_Success(t *testing.T) {
	expected := &MirrorConfig{
		IsEnabled:         true,
		ExternalReference: "registry.example.com/repo",
		SyncInterval:      86400,
		SyncStartDate:     "2023-01-01T00:00:00Z",
		RobotUsername:     "org+robot",
		RootRule: RootRule{
			RuleKind:  "tag_glob_csv",
			RuleValue: []string{"latest", "v1.*"},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/repository/testns/testrepo/mirror" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Errorf("unexpected method: %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("unexpected auth header: %s", r.Header.Get("Authorization"))
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(expected)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	result, err := client.GetMirror("testns", "testrepo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
	if result.ExternalReference != expected.ExternalReference {
		t.Errorf("ExternalReference = %q, want %q", result.ExternalReference, expected.ExternalReference)
	}
	if result.SyncInterval != expected.SyncInterval {
		t.Errorf("SyncInterval = %d, want %d", result.SyncInterval, expected.SyncInterval)
	}
	if !result.IsEnabled {
		t.Error("expected IsEnabled to be true")
	}
}

func TestGetMirror_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	result, err := client.GetMirror("testns", "testrepo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Fatalf("expected nil result for 404, got %+v", result)
	}
}

func TestCreateMirror_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/repository/testns/testrepo/mirror" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
		}

		var req MirrorCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if req.ExternalReference != "registry.example.com/repo" {
			t.Errorf("ExternalReference = %q, want %q", req.ExternalReference, "registry.example.com/repo")
		}

		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	err := client.CreateMirror("testns", "testrepo", &MirrorCreateRequest{
		IsEnabled:         true,
		ExternalReference: "registry.example.com/repo",
		SyncInterval:      86400,
		SyncStartDate:     "2023-01-01T00:00:00Z",
		RobotUsername:     "org+robot",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreateMirror_Conflict(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusConflict)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	err := client.CreateMirror("testns", "testrepo", &MirrorCreateRequest{
		ExternalReference: "registry.example.com/repo",
		SyncInterval:      86400,
		SyncStartDate:     "2023-01-01T00:00:00Z",
		RobotUsername:     "org+robot",
	})
	if err == nil {
		t.Fatal("expected error for conflict")
	}
}

func TestUpdateMirror_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/repository/testns/testrepo/mirror" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPut {
			t.Errorf("unexpected method: %s", r.Method)
		}

		var req MirrorUpdateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	isEnabled := true
	client := NewClient(server.URL, "test-token")
	err := client.UpdateMirror("testns", "testrepo", &MirrorUpdateRequest{
		IsEnabled: &isEnabled,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSyncNow_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/repository/testns/testrepo/mirror/sync-now" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("unexpected method: %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token")
	err := client.SyncNow("testns", "testrepo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
