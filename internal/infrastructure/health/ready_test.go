package health

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestReadyReusesConnectionAfterReadingBody(t *testing.T) {
	var opened atomic.Int64
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"status":"ready"}`))
	}))
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			opened.Add(1)
		}
	}
	server.Start()
	defer server.Close()
	client := NewHTTPReady(server.URL)
	defer client.Close()
	for range 10 {
		if !client.Ready(context.Background()) {
			t.Fatal("healthy server rejected")
		}
	}
	if count := opened.Load(); count != 1 {
		t.Fatalf("10 sequential probes opened %d connections; response body was not drained", count)
	}
}

func TestReadyStopsReadingOnCancellation(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		close(started)
		<-req.Context().Done()
	}))
	defer server.Close()
	client := NewHTTPReady(server.URL)
	defer client.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan bool, 1)
	go func() { done <- client.Ready(ctx) }()
	<-started
	cancel()
	if <-done {
		t.Fatal("cancelled probe reported ready")
	}
}
