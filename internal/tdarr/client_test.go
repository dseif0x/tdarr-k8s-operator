package tdarr

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStatusParsing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/cruddb":
			// table1Count = transcode queue, table4Count = health check queue.
			// table4Count is returned as a string to exercise coercion.
			_, _ = w.Write([]byte(`{"table1Count":3,"table2Count":10,"table4Count":"2"}`))
		case "/api/v2/get-nodes":
			_, _ = w.Write([]byte(`{
				"nodeA":{"nodeName":"a","workers":{"w1":{"idle":false},"w2":{"idle":true}}},
				"nodeB":{"nodeName":"b","workers":{"w3":{"idle":true}}}
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := New(srv.URL)
	st, err := c.Status(context.Background(), "table1Count", "table4Count")
	if err != nil {
		t.Fatalf("Status returned error: %v", err)
	}
	if st.TranscodeQueue != 3 {
		t.Errorf("TranscodeQueue = %d, want 3", st.TranscodeQueue)
	}
	if st.HealthCheckQueue != 2 {
		t.Errorf("HealthCheckQueue = %d, want 2", st.HealthCheckQueue)
	}
	if st.ActiveWorkers != 1 {
		t.Errorf("ActiveWorkers = %d, want 1", st.ActiveWorkers)
	}
	if !st.Pending() {
		t.Errorf("Pending() = false, want true")
	}
}

func TestStatusEmptyQueueIdleWorkers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/cruddb":
			_, _ = w.Write([]byte(`{"table1Count":0,"table4Count":0}`))
		case "/api/v2/get-nodes":
			_, _ = w.Write([]byte(`{"nodeA":{"nodeName":"a","workers":{}}}`))
		}
	}))
	defer srv.Close()

	st, err := New(srv.URL).Status(context.Background(), "table1Count", "table4Count")
	if err != nil {
		t.Fatalf("Status returned error: %v", err)
	}
	if st.Pending() {
		t.Errorf("Pending() = true, want false for empty queue and no active workers")
	}
}

func TestWorkerActiveFallback(t *testing.T) {
	cases := []struct {
		name string
		w    worker
		want bool
	}{
		{"explicit idle false", worker{Idle: ptr(false)}, true},
		{"explicit idle true", worker{Idle: ptr(true)}, false},
		{"no idle but file set", worker{File: "/media/x.mkv"}, true},
		{"no idle, status idle", worker{Status: "Idle"}, false},
		{"no idle, status transcoding", worker{Status: "transcoding"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.w.active(); got != tc.want {
				t.Errorf("active() = %v, want %v", got, tc.want)
			}
		})
	}
}

func ptr[T any](v T) *T { return &v }
