// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

package sfu

import (
	"bytes"
	"sync"
	"testing"

	"github.com/janickiy/meet-space/internal/domain/media"
	"github.com/pion/rtp"
)

func packetRecordingFixture() (*Manager, *publishedTrack, *egress, *rtp.Packet) {
	m := &Manager{}
	r := &room{egresses: map[string]*egress{}}
	t := &publishedTrack{metadata: media.Track{ID: "track"}, publisher: &peer{room: r}}
	t.permitted.Store(true)
	e := &egress{manager: m, room: r, id: "recording", tracks: map[string]bool{"track": true}, frames: make(chan media.EgressFrame, 1), done: make(chan struct{})}
	r.egresses[e.id] = e
	packet := &rtp.Packet{Header: rtp.Header{Version: 2, SequenceNumber: 12, Timestamp: 90000, SSRC: 1234}, Payload: bytes.Repeat([]byte{42}, 1200)}
	return m, t, e, packet
}

func TestRecordPacketConcurrentClose(t *testing.T) {
	for range 100 {
		m, track, e, packet := packetRecordingFixture()
		var workers sync.WaitGroup
		workers.Add(1)
		go func() {
			defer workers.Done()
			for range 100 {
				m.recordPacket(track, packet)
			}
		}()
		e.Close()
		workers.Wait()
		select {
		case <-e.Done():
		default:
			t.Fatal("recording did not close")
		}
		if len(e.room.egresses) != 0 {
			t.Fatal("closed recording retained ownership")
		}
	}
}

func TestRecordPacketPreservesMediaAndOwnership(t *testing.T) {
	m, track, e, packet := packetRecordingFixture()
	want, err := packet.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	m.recordPacket(track, packet)
	packet.Payload[0] = 99
	frame := <-e.frames
	if frame.Type != "rtp" || frame.TrackID != "track" || frame.Sequence != 1 || frame.CapturedAt <= 0 || !bytes.Equal(frame.RTP, want) {
		t.Fatalf("recording did not own an intact RTP frame: %+v", frame)
	}
	track.permitted.Store(false)
	m.recordPacket(track, packet)
	if len(e.frames) != 0 {
		t.Fatal("revoked media reached recording")
	}
}

// BenchmarkRecordPacket измеряет реальный путь записи RTP (1 200 байт),
// включая сериализацию, постановку в очередь и чтение ограниченной очереди.
func BenchmarkRecordPacket(b *testing.B) {
	for _, active := range []bool{false, true} {
		name := "inactive"
		if active {
			name = "active"
		}
		b.Run(name, func(b *testing.B) {
			m, track, e, packet := packetRecordingFixture()
			if !active {
				delete(track.publisher.room.egresses, e.id)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				m.recordPacket(track, packet)
				if active {
					<-e.frames
				}
			}
		})
	}
}
