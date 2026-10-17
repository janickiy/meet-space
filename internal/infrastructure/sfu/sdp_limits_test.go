package sfu

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/domain/media"
	pion "github.com/pion/webrtc/v4"
)

// Model Firefox's forty bundled receive slots without user identifiers or ICE secrets.
func fortySlotOffer(size int) string {
	var mids []string
	for i := 0; i < 40; i++ {
		mids = append(mids, fmt.Sprint(i))
	}
	raw := "v=0\r\no=- 1 1 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\na=group:BUNDLE " + strings.Join(mids, " ") + "\r\n"
	for i := 0; i < 40; i++ {
		kind, codec, payload := "audio", "opus/48000/2", 111
		if i%2 == 1 {
			kind, codec, payload = "video", "VP8/90000", 96
		}
		port := 0
		if i == 0 {
			port = 9
		}
		raw += fmt.Sprintf("m=%s %d UDP/TLS/RTP/SAVPF %d\r\na=mid:%d\r\na=recvonly\r\na=rtpmap:%d %s\r\n", kind, port, payload, i, payload, codec)
		if port == 0 {
			raw += "a=bundle-only\r\n"
		}
	}
	const padding = "a=x-padding:"
	raw += padding + strings.Repeat("x", size-len(raw)-len(padding)-2) + "\r\n"
	return raw
}

func TestFortySlotSDPLimits(t *testing.T) {
	for _, size := range []int{49152, 49153, 65535, 65536, 65537, media.MaxSDPBytes - 1, media.MaxSDPBytes, media.MaxSDPBytes + 1} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			raw := fortySlotOffer(size)
			if len(raw) != size {
				t.Fatalf("fixture size=%d want=%d", len(raw), size)
			}
			sources, err := validateSourceOffer(raw, 10, []media.Publication{}, 4, 2, 2)
			if size <= media.MaxSDPBytes {
				if err != nil || len(sources) != 0 {
					t.Fatalf("valid forty-slot offer rejected: %v", err)
				}
			} else if !errors.Is(err, media.ErrInvalid) {
				t.Fatalf("oversized SDP accepted: %v", err)
			}
		})
	}
}

func TestLargeSDPPreservesPublicationAndBundleChecks(t *testing.T) {
	raw := fortySlotOffer(51212)
	for _, tc := range []struct {
		name, raw string
		pubs      []media.Publication
		want      error
	}{
		{"legacy missing declarations", raw, nil, media.ErrLimit},
		{"undeclared publisher", strings.Replace(raw, "a=recvonly", "a=sendrecv", 1), []media.Publication{}, media.ErrInvalid},
		{"unknown bundle member", strings.Replace(raw, "BUNDLE 0 1", "BUNDLE 999 1", 1), []media.Publication{}, media.ErrInvalid},
		{"duplicate mid", strings.Replace(raw, "a=mid:1\r\n", "a=mid:0\r\n", 1), []media.Publication{}, media.ErrInvalid},
		{"unsupported publication codec", strings.Replace(strings.Replace(raw, "a=recvonly", "a=sendrecv", 1), "opus/48000/2", "PCMU/8000", 1), []media.Publication{{MID: "0", Source: media.SourceMicrophone}}, media.ErrInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := validateSourceOffer(tc.raw, 10, tc.pubs, 4, 2, 2); !errors.Is(err, tc.want) {
				t.Fatalf("got %v want %v", err, tc.want)
			}
		})
	}
}

func TestLargePionOfferNegotiatesFortySlots(t *testing.T) {
	manager, err := NewManager(Options{MaxPeers: 10, MaxRooms: 1, NegotiationTimeout: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := manager.Shutdown(ctx); err != nil {
			t.Error(err)
		}
	})
	binding := media.Binding{ConferenceID: uuid.NewString(), ParticipantID: uuid.NewString(), SessionID: uuid.NewString(), ConnectionID: uuid.NewString(), UserID: uuid.NewString(), AuthorizationExpiresAt: time.Now().Add(time.Hour)}
	peer, err := manager.Join(context.Background(), binding)
	if err != nil {
		t.Fatal(err)
	}
	pc, err := pion.NewPeerConnection(pion.Configuration{BundlePolicy: pion.BundlePolicyMaxBundle})
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()
	for i := 0; i < 20; i++ {
		for _, kind := range []pion.RTPCodecType{pion.RTPCodecTypeAudio, pion.RTPCodecTypeVideo} {
			if _, err := pc.AddTransceiverFromKind(kind, pion.RTPTransceiverInit{Direction: pion.RTPTransceiverDirectionRecvonly}); err != nil {
				t.Fatal(err)
			}
		}
	}
	offer, err := pc.CreateOffer(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(offer.SDP) <= 49152 || len(offer.SDP) > media.MaxSDPBytes {
		t.Fatalf("fixture SDP outside regression range: %d", len(offer.SDP))
	}
	if err = pc.SetLocalDescription(offer); err != nil {
		t.Fatal(err)
	}
	negotiation := uuid.NewString()
	answer, err := manager.OfferSources(context.Background(), peer.MediaPeerID, negotiation, offer.SDP, []media.Publication{})
	if err != nil {
		t.Fatal(err)
	}
	if err = pc.SetRemoteDescription(answer); err != nil {
		t.Fatal(err)
	}
	if err = manager.Ready(context.Background(), peer.MediaPeerID, negotiation); err != nil {
		t.Fatal(err)
	}
}
