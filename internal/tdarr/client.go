// Package tdarr is a minimal client for the Tdarr server HTTP API. It only
// implements the read-only endpoints the operator needs to decide whether a
// transcode node is required.
package tdarr

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client talks to a Tdarr server.
type Client struct {
	baseURL string
	http    *http.Client
}

// New returns a Client for the given Tdarr server base URL
// (e.g. http://my-release-tdarr:8265).
func New(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 15 * time.Second},
	}
}

// QueueStatus summarises the pending work on the server.
type QueueStatus struct {
	TranscodeQueue   int
	HealthCheckQueue int
	// ActiveWorkers is the number of connected node workers that are
	// currently busy. A file that is mid-transcode no longer appears in the
	// transcode queue, so this is required to avoid tearing down a node while
	// it is still working.
	ActiveWorkers int
}

// Pending reports whether there is any work in progress or waiting.
func (q QueueStatus) Pending() bool {
	return q.TranscodeQueue > 0 || q.HealthCheckQueue > 0 || q.ActiveWorkers > 0
}

// crudDBRequest is the body shape for the /api/v2/cruddb endpoint.
type crudDBRequest struct {
	Data crudDBData `json:"data"`
}

type crudDBData struct {
	Collection string `json:"collection"`
	Mode       string `json:"mode"`
	DocID      string `json:"docID"`
}

// Statistics fetches the StatisticsJSONDB document and returns the configured
// transcode/health-check queue counts. The document is parsed loosely into a
// map so we tolerate Tdarr schema differences across versions.
func (c *Client) Statistics(ctx context.Context, transcodeField, healthCheckField string) (transcode, healthCheck int, err error) {
	reqBody := crudDBRequest{Data: crudDBData{
		Collection: "StatisticsJSONDB",
		Mode:       "getById",
		DocID:      "statistics",
	}}
	raw, err := c.post(ctx, "/api/v2/cruddb", reqBody)
	if err != nil {
		return 0, 0, err
	}

	var stats map[string]json.RawMessage
	if err := json.Unmarshal(raw, &stats); err != nil {
		return 0, 0, fmt.Errorf("decode statistics: %w", err)
	}
	return asInt(stats[transcodeField]), asInt(stats[healthCheckField]), nil
}

// node mirrors the parts of a /api/v2/get-nodes entry we care about.
type node struct {
	NodeName string            `json:"nodeName"`
	Workers  map[string]worker `json:"workers"`
}

type worker struct {
	Idle   *bool  `json:"idle"`
	Status string `json:"status"`
	File   string `json:"file"`
}

func (w worker) active() bool {
	if w.Idle != nil {
		return !*w.Idle
	}
	// Fall back to heuristics when the idle flag is absent.
	return w.File != "" || (w.Status != "" && !strings.EqualFold(w.Status, "idle"))
}

// ActiveWorkers returns the number of busy workers across all connected nodes.
func (c *Client) ActiveWorkers(ctx context.Context) (int, error) {
	raw, err := c.get(ctx, "/api/v2/get-nodes")
	if err != nil {
		return 0, err
	}
	var nodes map[string]node
	if err := json.Unmarshal(raw, &nodes); err != nil {
		return 0, fmt.Errorf("decode get-nodes: %w", err)
	}
	active := 0
	for _, n := range nodes {
		for _, w := range n.Workers {
			if w.active() {
				active++
			}
		}
	}
	return active, nil
}

// Status combines the queue counts and active worker count into a QueueStatus.
func (c *Client) Status(ctx context.Context, transcodeField, healthCheckField string) (QueueStatus, error) {
	transcode, healthCheck, err := c.Statistics(ctx, transcodeField, healthCheckField)
	if err != nil {
		return QueueStatus{}, err
	}
	active, err := c.ActiveWorkers(ctx)
	if err != nil {
		return QueueStatus{}, err
	}
	return QueueStatus{
		TranscodeQueue:   transcode,
		HealthCheckQueue: healthCheck,
		ActiveWorkers:    active,
	}, nil
}

func (c *Client) post(ctx context.Context, path string, body any) ([]byte, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(req)
}

func (c *Client) get(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	return c.do(req)
}

func (c *Client) do(req *http.Request) ([]byte, error) {
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("tdarr %s %s: status %d: %s", req.Method, req.URL.Path, resp.StatusCode, truncate(string(data), 256))
	}
	return data, nil
}

// asInt coerces a JSON value that may be a number or a numeric string into an
// int, returning 0 when absent or unparseable.
func asInt(raw json.RawMessage) int {
	if len(raw) == 0 {
		return 0
	}
	var n float64
	if err := json.Unmarshal(raw, &n); err == nil {
		return int(n)
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		var f float64
		if _, err := fmt.Sscanf(s, "%f", &f); err == nil {
			return int(f)
		}
	}
	return 0
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
