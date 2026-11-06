// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package sfu

import (
	"net"
	"sync"
)

// ownedTCPListener закрывает также соединения, ещё не переданные в PeerConnection:
// ожидающие первого STUN-пакета и соединения с неизвестным ICE username.
type ownedTCPListener struct {
	net.Listener
	mu          sync.Mutex
	closed      bool
	connections map[*ownedTCPConn]struct{}
}

type ownedTCPConn struct {
	net.Conn
	owner *ownedTCPListener
}

func (l *ownedTCPListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		_ = conn.Close()
		return nil, net.ErrClosed
	}
	owned := &ownedTCPConn{Conn: conn, owner: l}
	l.connections[owned] = struct{}{}
	return owned, nil
}

func (l *ownedTCPListener) Close() error {
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return nil
	}
	l.closed = true
	connections := make([]*ownedTCPConn, 0, len(l.connections))
	for conn := range l.connections {
		connections = append(connections, conn)
	}
	l.mu.Unlock()
	err := l.Listener.Close()
	for _, conn := range connections {
		_ = conn.Close()
	}
	return err
}

func (c *ownedTCPConn) Close() error {
	err := c.Conn.Close()
	c.owner.mu.Lock()
	delete(c.owner.connections, c)
	c.owner.mu.Unlock()
	return err
}
