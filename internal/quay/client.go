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
	"crypto/tls"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const httpTimeout = 30 * time.Second

// ClientOption is a functional option for configuring the Quay API client.
type ClientOption func(*Client)

// Client is a Quay Container Registry API client.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewClient creates a new Quay API client.
func NewClient(baseURL, token string, opts ...ClientOption) *Client {
	c := &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		httpClient: &http.Client{
			Timeout: httpTimeout,
		},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// WithInsecureSkipVerify returns a ClientOption that disables TLS certificate verification.
func WithInsecureSkipVerify() ClientOption {
	return func(c *Client) {
		c.httpClient = &http.Client{
			Timeout: httpTimeout,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true, //nolint:gosec // user-configured for self-signed certs
				},
			},
		}
	}
}

// WithHTTPClient returns a ClientOption that sets a custom HTTP client.
func WithHTTPClient(httpClient *http.Client) ClientOption {
	return func(c *Client) {
		c.httpClient = httpClient
	}
}

func (c *Client) newRequest(ctx context.Context, method, path string, body *strings.Reader) (*http.Request, error) {
	url := c.baseURL + path
	var req *http.Request
	var err error
	if body != nil {
		req, err = http.NewRequestWithContext(ctx, method, url, body)
	} else {
		req, err = http.NewRequestWithContext(ctx, method, url, nil)
	}
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}

// ParseSyncInterval converts a sync interval string (with s/m/h/d/w suffix) to seconds.
// Bare numbers are treated as seconds.
func ParseSyncInterval(interval string) (int, error) {
	if interval == "" {
		return 86400, nil
	}

	interval = strings.TrimSpace(interval)
	if len(interval) == 0 {
		return 86400, nil
	}

	// Check if last character is a suffix
	last := interval[len(interval)-1]
	switch {
	case last >= '0' && last <= '9':
		// Bare number, treat as seconds
		val, err := strconv.Atoi(interval)
		if err != nil {
			return 0, fmt.Errorf("invalid sync interval %q: %w", interval, err)
		}
		return val, nil
	default:
		numStr := interval[:len(interval)-1]
		val, err := strconv.Atoi(numStr)
		if err != nil {
			return 0, fmt.Errorf("invalid sync interval %q: %w", interval, err)
		}
		switch last {
		case 's':
			return val, nil
		case 'm':
			return val * 60, nil
		case 'h':
			return val * 3600, nil
		case 'd':
			return val * 86400, nil
		case 'w':
			return val * 604800, nil
		default:
			return 0, fmt.Errorf("invalid sync interval suffix %q in %q", string(last), interval)
		}
	}
}
