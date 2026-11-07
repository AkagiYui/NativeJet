package core

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"nativejet/internal/model"
	"nativejet/internal/platform"
)

func TestTrafficDisconnectReconnectAndIdle(t *testing.T) {
	var streams atomic.Int32
	disconnect := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/traffic" || r.Header.Get("Authorization") != "Bearer test-token" {
			http.Error(w, "wrong controller request", http.StatusBadRequest)
			return
		}
		n := streams.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if n == 1 {
			fmt.Fprintln(w, `{"up":1024,"down":2048,"upTotal":222222,"downTotal":999999}`)
			w.(http.Flusher).Flush()
			select {
			case <-disconnect:
			case <-r.Context().Done():
			}
		} else if n == 2 {
			fmt.Fprintln(w, `{"up":0,"down":0}`)
			w.(http.Flusher).Flush()
			<-r.Context().Done() // No new sample; keep the transport open.
		} else {
			// A partial frame on reconnect must not appear as a valid zero rate.
			fmt.Fprintln(w, `{"up":0}`)
			w.(http.Flusher).Flush()
			<-r.Context().Done()
		}
	}))
	defer server.Close()
	pid := os.Getpid()
	path, stamp, err := platform.ProcessIdentity(pid)
	if err != nil {
		t.Fatal(err)
	}
	record := &model.ProcessRecord{PID: pid, Path: path, StartTime: stamp}
	supervisor := &Supervisor{record: record}
	controller := ConfiguredController(map[string]any{"external-controller": server.Listener.Addr().String(), "secret": "test-token"}, "")
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		supervisor.followTraffic(ctx, controller, record)
	}()
	defer func() { cancel(); <-done }()

	wait := func(want model.Traffic, timeout time.Duration) {
		t.Helper()
		deadline := time.Now().Add(timeout)
		for {
			got := supervisor.Traffic(false).Traffic
			if got == want {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("traffic = %+v, want %+v (stream count %d)", got, want, streams.Load())
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	wait(model.Traffic{Available: true, Up: 1024, Down: 2048}, 3*time.Second)
	close(disconnect)
	wait(model.Traffic{}, 2*time.Second)
	wait(model.Traffic{Available: true}, 3*time.Second)
	wait(model.Traffic{}, trafficIdleTimeout+time.Second)
	history := supervisor.Traffic(true).History
	if len(history) != 2 || history[0].Down != 2048 || history[1].Down != 0 ||
		!history[1].BreakBefore || !history[0].At.Before(history[1].At) {
		t.Fatalf("expected real samples and a break across the reconnect, got %+v", history)
	}
	deadline := time.Now().Add(3 * time.Second)
	for streams.Load() < 3 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if streams.Load() < 3 {
		t.Fatal("the stalled stream was not reopened")
	}
	if got := supervisor.Traffic(false); got.Available {
		t.Fatalf("a partial frame produced a valid sample: %+v", got)
	}
}

func TestTrafficHistoryReturnsOnlyRecentSamplesAfterWrapping(t *testing.T) {
	var h trafficHistory
	start := time.Unix(1000, 0)
	for i := range 2 * trafficHistoryCapacity {
		h.add(model.TrafficPoint{At: start.Add(time.Duration(i) * time.Second), Up: int64(i)})
	}
	last := start.Add(time.Duration(2*trafficHistoryCapacity-1) * time.Second)
	points := h.since(last.Add(-model.TrafficHistoryWindow))
	if len(points) != 301 || points[0].At != last.Add(-5*time.Minute) || points[len(points)-1].At != last {
		t.Fatalf("history did not retain the five minute window in order: %d samples", len(points))
	}
	if expired := h.since(last.Add(time.Second)); len(expired) != 0 {
		t.Fatalf("history returned %d expired samples", len(expired))
	}
}
