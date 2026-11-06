// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package sfu

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"log/slog"
	"net"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pion/stun/v3"
)

type tcpLifecycleLog struct {
	slog.Handler
	info       atomic.Int32
	registered chan struct{}
}

func (l *tcpLifecycleLog) Handle(_ context.Context, record slog.Record) error {
	// Pion сообщает о запуске listener, затем о добавлении STUN-соединения.
	if record.Level == slog.LevelInfo && l.info.Add(1) == 2 {
		close(l.registered)
	}
	return nil
}

func TestShutdownClosesUnclaimedTCPConnection(t *testing.T) {
	reserved, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := reserved.Addr().(*net.TCPAddr).Port
	_ = reserved.Close()
	logger := &tcpLifecycleLog{Handler: slog.NewTextHandler(io.Discard, nil), registered: make(chan struct{})}
	m, err := NewManager(Options{TCPPort: port, Logger: slog.New(logger)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = m.Shutdown(ctx)
	})
	conn, err := net.DialTimeout("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	msg := stun.MustBuild(stun.TransactionID, stun.BindingRequest, stun.NewUsername("unclaimed:remote"))
	frame := make([]byte, 2+len(msg.Raw))
	binary.BigEndian.PutUint16(frame, uint16(len(msg.Raw)))
	copy(frame[2:], msg.Raw)
	if _, err := conn.Write(frame); err != nil {
		t.Fatal(err)
	}
	select {
	case <-logger.registered:
	case <-time.After(time.Second):
		t.Fatal("TCP STUN connection was not registered")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := m.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(250 * time.Millisecond))
	var data [1]byte
	if _, err := conn.Read(data[:]); !errors.Is(err, io.EOF) {
		t.Fatalf("connection survived successful shutdown: %v", err)
	}
}

func TestTCPListenerClosesIdleConnections(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	owned := &ownedTCPListener{Listener: listener, connections: map[*ownedTCPConn]struct{}{}}
	defer owned.Close()
	client, err := net.DialTimeout("tcp4", listener.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	server, err := owned.Accept()
	if err != nil {
		t.Fatal(err)
	}
	// Ни одного байта STUN: shutdown должен прервать уже начатое чтение.
	readDone := make(chan error, 1)
	go func() { var data [1]byte; _, err := server.Read(data[:]); readDone <- err }()
	if err := owned.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-readDone:
		if !errors.Is(err, net.ErrClosed) {
			t.Fatalf("idle read: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("idle read survived listener close")
	}
	_ = client.SetReadDeadline(time.Now().Add(time.Second))
	var data [1]byte
	if _, err := client.Read(data[:]); !errors.Is(err, io.EOF) {
		t.Fatalf("idle connection survived: %v", err)
	}
	owned.mu.Lock()
	remaining := len(owned.connections)
	owned.mu.Unlock()
	if remaining != 0 {
		t.Fatalf("retained %d connections", remaining)
	}
}

func TestTCPConnectionCloseReleasesOwnership(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	owned := &ownedTCPListener{Listener: listener, connections: map[*ownedTCPConn]struct{}{}}
	defer owned.Close()
	for range 50 {
		client, err := net.DialTimeout("tcp4", listener.Addr().String(), time.Second)
		if err != nil {
			t.Fatal(err)
		}
		server, err := owned.Accept()
		if err != nil {
			client.Close()
			t.Fatal(err)
		}
		_ = server.Close()
		_ = client.Close()
	}
	owned.mu.Lock()
	remaining := len(owned.connections)
	owned.mu.Unlock()
	if remaining != 0 {
		t.Fatalf("retained %d closed connections", remaining)
	}
}
