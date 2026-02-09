// Package main implements a minimal Quay mirror API mock for e2e testing.
// It supports the 4 endpoints used by the quay-config-operator controller:
//   - GET    /api/v1/repository/{ns}/{repo}/mirror
//   - POST   /api/v1/repository/{ns}/{repo}/mirror
//   - PUT    /api/v1/repository/{ns}/{repo}/mirror
//   - POST   /api/v1/repository/{ns}/{repo}/mirror/sync-now
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
)

type mirrorConfig struct {
	IsEnabled              bool                    `json:"is_enabled"`
	ExternalReference      string                  `json:"external_reference"`
	ExternalRegistryConfig *externalRegistryConfig `json:"external_registry_config,omitempty"`
	SyncInterval           int                     `json:"sync_interval"`
	SyncStartDate          string                  `json:"sync_start_date"`
	RobotUsername          string                  `json:"robot_username"`
	RootRule               *rootRule               `json:"root_rule,omitempty"`
}

type externalRegistryConfig struct {
	VerifyTLS      *bool  `json:"verify_tls,omitempty"`
	UnsignedImages *bool  `json:"unsigned_images,omitempty"`
	Proxy          *proxy `json:"proxy,omitempty"`
}

type proxy struct {
	HTTPProxy  string `json:"http_proxy,omitempty"`
	HTTPSProxy string `json:"https_proxy,omitempty"`
	NoProxy    string `json:"no_proxy,omitempty"`
}

type rootRule struct {
	RuleKind  string   `json:"rule_kind"`
	RuleValue []string `json:"rule_value"`
}

type store struct {
	mu      sync.RWMutex
	mirrors map[string]*mirrorConfig // key: "namespace/repo"
}

func newStore() *store {
	return &store{mirrors: make(map[string]*mirrorConfig)}
}

func main() {
	token := os.Getenv("QUAY_TOKEN")
	if token == "" {
		token = "test-token"
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	s := newStore()

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Health check (unauthenticated)
		if path == "/health" || path == "/" {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, "ok")
			return
		}

		// Auth check for API endpoints
		auth := r.Header.Get("Authorization")
		if auth != "Bearer "+token {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}

		// POST /api/v1/repository/{ns}/{repo}/mirror/sync-now
		if r.Method == http.MethodPost && strings.HasSuffix(path, "/mirror/sync-now") {
			repoKey := extractRepoKey(strings.TrimSuffix(path, "/mirror/sync-now"))
			if repoKey == "" {
				http.Error(w, `{"error":"invalid path"}`, http.StatusBadRequest)
				return
			}
			log.Printf("SYNC-NOW %s", repoKey)
			w.WriteHeader(http.StatusNoContent)
			return
		}

		// GET/POST/PUT /api/v1/repository/{ns}/{repo}/mirror
		if strings.HasSuffix(path, "/mirror") {
			repoKey := extractRepoKey(strings.TrimSuffix(path, "/mirror"))
			if repoKey == "" {
				http.Error(w, `{"error":"invalid path"}`, http.StatusBadRequest)
				return
			}

			switch r.Method {
			case http.MethodGet:
				s.handleGet(w, repoKey)
			case http.MethodPost:
				s.handleCreate(w, r, repoKey)
			case http.MethodPut:
				s.handleUpdate(w, r, repoKey)
			default:
				http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			}
			return
		}

		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
	})

	addr := ":" + port
	log.Printf("quay-mock starting on %s (token=%s...)", addr, token[:min(8, len(token))])
	log.Fatal(http.ListenAndServe(addr, mux))
}

// extractRepoKey extracts "namespace/repo" from a path like
// "/api/v1/repository/namespace/repo".
func extractRepoKey(path string) string {
	const prefix = "/api/v1/repository/"
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	rest := path[len(prefix):]
	parts := strings.SplitN(rest, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return ""
	}
	return parts[0] + "/" + parts[1]
}

func (s *store) handleGet(w http.ResponseWriter, repoKey string) {
	s.mu.RLock()
	cfg, ok := s.mirrors[repoKey]
	s.mu.RUnlock()

	if !ok {
		log.Printf("GET %s -> 404", repoKey)
		w.WriteHeader(http.StatusNotFound)
		return
	}

	log.Printf("GET %s -> 200", repoKey)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(cfg)
}

func (s *store) handleCreate(w http.ResponseWriter, r *http.Request, repoKey string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.mirrors[repoKey]; ok {
		log.Printf("POST %s -> 409 (already exists)", repoKey)
		http.Error(w, `{"error":"mirror already exists"}`, http.StatusConflict)
		return
	}

	var cfg mirrorConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
		return
	}

	s.mirrors[repoKey] = &cfg
	log.Printf("POST %s -> 201", repoKey)
	w.WriteHeader(http.StatusCreated)
}

func (s *store) handleUpdate(w http.ResponseWriter, r *http.Request, repoKey string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var update map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
		return
	}

	existing, ok := s.mirrors[repoKey]
	if !ok {
		// Accept updates even if not created yet (Quay API allows this)
		existing = &mirrorConfig{}
		s.mirrors[repoKey] = existing
	}

	// Apply updates by re-marshaling through JSON for simplicity
	data, _ := json.Marshal(existing)
	_ = json.Unmarshal(data, &update)

	// Now marshal the merged update back to our struct
	merged, _ := json.Marshal(update)
	_ = json.Unmarshal(merged, existing)

	log.Printf("PUT %s -> 200", repoKey)
	w.WriteHeader(http.StatusOK)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
