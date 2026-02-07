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
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// GetMirror retrieves the mirror configuration for a repository.
// Returns nil, nil if the mirror does not exist (404).
func (c *Client) GetMirror(ctx context.Context, namespace, repo string) (*MirrorConfig, error) {
	path := fmt.Sprintf("/api/v1/repository/%s/%s/mirror", namespace, repo)
	req, err := c.newRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var config MirrorConfig
	if err := json.NewDecoder(resp.Body).Decode(&config); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &config, nil
}

// CreateMirror creates a mirror configuration for a repository.
func (c *Client) CreateMirror(ctx context.Context, namespace, repo string, createReq *MirrorCreateRequest) error {
	path := fmt.Sprintf("/api/v1/repository/%s/%s/mirror", namespace, repo)
	body, err := json.Marshal(createReq)
	if err != nil {
		return fmt.Errorf("marshaling request: %w", err)
	}

	req, err := c.newRequest(ctx, http.MethodPost, path, strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusConflict {
		return fmt.Errorf("mirror already exists for %s/%s", namespace, repo)
	}

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// UpdateMirror updates the mirror configuration for a repository.
func (c *Client) UpdateMirror(ctx context.Context, namespace, repo string, updateReq *MirrorUpdateRequest) error {
	path := fmt.Sprintf("/api/v1/repository/%s/%s/mirror", namespace, repo)
	body, err := json.Marshal(updateReq)
	if err != nil {
		return fmt.Errorf("marshaling request: %w", err)
	}

	req, err := c.newRequest(ctx, http.MethodPut, path, strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// SyncNow triggers an immediate synchronization of the repository mirror.
func (c *Client) SyncNow(ctx context.Context, namespace, repo string) error {
	path := fmt.Sprintf("/api/v1/repository/%s/%s/mirror/sync-now", namespace, repo)
	req, err := c.newRequest(ctx, http.MethodPost, path, strings.NewReader("{}"))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}
