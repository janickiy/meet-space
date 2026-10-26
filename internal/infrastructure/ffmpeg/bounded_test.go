package ffmpeg

import (
	"bytes"
	"context"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"
)

// TestRecorderProcessHelper is a synthetic subprocess, not an FFmpeg codec test.
// The explicit environment flag prevents its process behavior in ordinary test runs.
func TestRecorderProcessHelper(t *testing.T) {
	switch os.Getenv("MEETSPACE_RECORDER_TEST_PROCESS") {
	case "output":
		_, _ = os.Stdout.Write(bytes.Repeat([]byte("x"), 4*maxStderrBytes))
		os.Exit(0)
	case "ignore-term":
		signal.Ignore(syscall.SIGTERM)
		if err := os.WriteFile(os.Getenv("MEETSPACE_RECORDER_TEST_READY"), nil, 0o600); err != nil {
			os.Exit(2)
		}
		for {
			time.Sleep(time.Second)
		}
	}
}

// TestRunBoundedCapsProcessOutput verifies that large child output is retained only as a tail.
func TestRunBoundedCapsProcessOutput(t *testing.T) {
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("MEETSPACE_RECORDER_TEST_PROCESS", "output")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	output, err := runBounded(ctx, binary, "-test.run=^TestRecorderProcessHelper$")
	if err != nil {
		t.Fatal(err)
	}
	if len(output) != maxStderrBytes || !bytes.Equal(output, bytes.Repeat([]byte("x"), maxStderrBytes)) {
		t.Fatalf("unexpected output tail length: %d", len(output))
	}
}

// TestRunBoundedKillsAndWaitsForUncooperativeProcess checks cancellation escalation.
// A readiness file ensures SIGTERM is ignored before cancellation is requested.
func TestRunBoundedKillsAndWaitsForUncooperativeProcess(t *testing.T) {
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ready := filepath.Join(t.TempDir(), "ready")
	t.Setenv("MEETSPACE_RECORDER_TEST_PROCESS", "ignore-term")
	t.Setenv("MEETSPACE_RECORDER_TEST_READY", ready)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := runBounded(ctx, binary, "-test.run=^TestRecorderProcessHelper$")
		done <- err
	}()
	readyDeadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		if time.Now().After(readyDeadline) {
			cancel()
			<-done
			t.Fatal("helper did not become ready")
		}
		time.Sleep(10 * time.Millisecond)
	}
	started := time.Now()
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancelled process returned success")
		}
		t.Logf("uncooperative process killed and waited after %s", time.Since(started))
	case <-time.After(7 * time.Second):
		t.Fatal("process outlived cancellation and kill deadline")
	}
}

// TestLogTailConcurrentWriters checks bounded diagnostics under concurrent stdout/stderr.
func TestLogTailConcurrentWriters(t *testing.T) {
	var tail logTail
	var writers sync.WaitGroup
	for range 4 {
		writers.Add(1)
		go func() {
			defer writers.Done()
			data := bytes.Repeat([]byte("x"), 1024)
			for range 100 {
				if n, err := tail.Write(data); n != len(data) || err != nil {
					t.Errorf("write: %d, %v", n, err)
				}
				if len(tail.String()) > maxStderrBytes {
					t.Error("diagnostic output exceeded its bound")
				}
			}
		}()
	}
	writers.Wait()
	if len(tail.String()) != maxStderrBytes {
		t.Fatal("diagnostic tail did not retain the expected number of bytes")
	}
}
