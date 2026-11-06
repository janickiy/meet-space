// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package realtime

import (
	"fmt"
	"testing"

	domain "github.com/janickiy/meet-space/internal/domain/realtime"
)

func entriesFixture(n int) *Hub {
	h := &Hub{local: make(map[string]*localSocket, n)}
	for i := range n {
		room := "other"
		if i < 2 {
			room = "target"
		}
		h.local[fmt.Sprint(i)] = &localSocket{session: domain.Session{ConferenceID: room}, ready: true}
	}
	return h
}

func TestEntriesOnlyIncludesReadySocketsInRoom(t *testing.T) {
	h := entriesFixture(100)
	h.local["pending"] = &localSocket{session: domain.Session{ConferenceID: "target"}}
	if got := h.entries("target"); len(got) != 2 {
		t.Fatalf("room snapshot has %d entries, want 2", len(got))
	}
	if got := h.entries("absent"); len(got) != 0 {
		t.Fatalf("unknown room has %d entries", len(got))
	}
	if got := h.entries(""); len(got) != 100 {
		t.Fatalf("global snapshot has %d entries, want 100", len(got))
	}
}

func BenchmarkEntriesSmallRoom(b *testing.B) {
	for _, n := range []int{100, 1000, 10000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			h := entriesFixture(n)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				if got := h.entries("target"); len(got) != 2 {
					b.Fatal(len(got))
				}
			}
		})
	}
}
