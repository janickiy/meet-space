package records

import "testing"

// TestIsSFURecording verifies processing requires a supported mode, source,
// transport, and matching conference identity without weakening read compatibility.
func TestIsSFURecording(t *testing.T) {
	conference := "0fbfa099-e4dc-4441-ad28-ec89a3c51f38"
	other := "970cf22e-8045-4d05-a947-d3dc49b9bb7b"
	base := Record{ConferenceID: conference, PlatformConferenceID: &conference, Mode: ModeComposite, SourceType: "conference", TransportType: "sfu"}
	for _, mode := range []string{ModeComposite, ModeAudioOnly, ModeIndividualTracks, ModeScreenFocus} {
		record := base
		record.Mode = mode
		if !IsSFURecording(record) {
			t.Fatalf("valid SFU mode %q rejected", mode)
		}
	}
	for name, mutate := range map[string]func(*Record){
		"legacy mode":        func(r *Record) { r.Mode = "legacy" },
		"unknown mode":       func(r *Record) { r.Mode = "unknown" },
		"direct source":      func(r *Record) { r.SourceType = "browser" },
		"direct transport":   func(r *Record) { r.TransportType = "webrtc" },
		"missing conference": func(r *Record) { r.PlatformConferenceID = nil },
		"other conference":   func(r *Record) { r.PlatformConferenceID = &other },
		"empty conference":   func(r *Record) { empty := ""; r.ConferenceID = empty; r.PlatformConferenceID = &empty },
	} {
		t.Run(name, func(t *testing.T) {
			record := base
			mutate(&record)
			if IsSFURecording(record) {
				t.Fatal("unsupported recording accepted")
			}
		})
	}
}
