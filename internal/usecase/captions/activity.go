package captions

import "sync"

// activityMeter подсчитывает объединение 20-миллисекундных аудиоинтервалов одного участника, а
// не сумму интервалов всех устройств.  Ограничение по памяти — 30 секунд на вызов и 500
// участников; игнорируйте поздние фреймы из старых интервалов.
type activityMeter struct {
	mu           sync.Mutex
	participants map[string]*activityWindow
}

// activityWindow хранит только биты наличия звука и активности, но не биты PCM или текстовые
// биты.
type activityWindow struct {
	latest int64
	slots  [1500]activitySlot
}

// activitySlot предотвращает повторный подсчет одного и того же временного интервала в повторно
// используемой ячейке вызова.
type activitySlot struct {
	bucket             int64
	observed, speaking bool
}

// observe объединяет активность из параллельных аудиоисточников на абсолютной временной шкале
// конференции.
//
// @Parameters:
//   - participant: Членство в конференции или публичное состояние участника.
//   - start: Начало временного интервала в единицах измерения, определенных сигнатурой.
//   - end: Конец временного интервала в единицах измерения, определенных сигнатурой.
//   - active: Содержат ли наблюдаемые аудиоинтервалы речевую активность.
//
// @returns:
//   - speech: Возвращает количество, размер или числовое значение.
//   - observed: Возвращает количество, размер или числовое значение.
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
