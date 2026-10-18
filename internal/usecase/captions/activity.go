package captions

import "sync"

// activityMeter counts the union of one participant's 20 ms audio intervals, not the sum of
// devices. A 30-second ring and 500-participant limit bound memory; ignore late frames from old
// intervals.
type activityMeter struct {
	mu           sync.Mutex
	participants map[string]*activityWindow
}

// activityWindow stores only audio-present and activity bits, never PCM or text.
type activityWindow struct {
	latest int64
	slots  [1500]activitySlot
}

// activitySlot prevents a reused ring cell from counting the same time interval twice.
type activitySlot struct {
	bucket             int64
	observed, speaking bool
}

// observe merges activity from parallel audio sources on the absolute conference timeline.
//
// Parameters:
//   - participant: Participant membership or public participant state.
//   - start: Start of the time interval in the units defined by the signature.
//   - end: End of the time interval in the units defined by the signature.
//   - active: Whether this observed audio interval contains speech activity.
//
// Returns:
//   - speech: The resulting count, size or numeric value.
//   - observed: The resulting count, size or numeric value.
func (m *activityMeter) observe(participant string, start, end int64, active bool) (speech, observed int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.participants == nil {
		m.participants = map[string]*activityWindow{}
	}
	w := m.participants[participant]
	if w == nil {
		if len(m.participants) >= 500 {
			return
		}
		w = &activityWindow{}
		m.participants[participant] = w
	}
	if start < 0 || end <= start || end-start > 1000 {
		return
	}
	for bucket := start / 20; bucket < end/20; bucket++ {
		if bucket < w.latest-1499 {
			continue
		}
		w.latest = max(w.latest, bucket)
		s := &w.slots[bucket%1500]
		if s.bucket != bucket {
			*s = activitySlot{bucket: bucket}
		}
		if !s.observed {
			s.observed = true
			observed += 20
		}
		if active && !s.speaking {
			s.speaking = true
			speech += 20
		}
	}
	return
}
