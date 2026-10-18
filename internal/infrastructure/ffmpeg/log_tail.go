package ffmpeg

import "sync"

const maxStderrBytes = 64 * 1024

// logTail сохраняет только ограниченный объем диагностических данных внешнего процесса.
type logTail struct {
	mu   sync.Mutex
	data []byte
}

// Функция Write принимает выходные байты через контракт io.Writer, соблюдая правила буферизации
// компонента.
//
// @Parameters:
//   - p: Входные байты, потребляемые этой операцией.
//
// @returns:
//   - result1: Возвращает количество, размер или числовое значение.
//   - result2: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (b *logTail) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := len(p)
	if n >= maxStderrBytes {
		b.data = append(b.data[:0], p[n-maxStderrBytes:]...)
		return n, nil
	}
	if overflow := len(b.data) + n - maxStderrBytes; overflow > 0 {
		b.data = b.data[:copy(b.data, b.data[overflow:])]
	}
	b.data = append(b.data, p...)
	return n, nil
}

// String возвращает буферизованный диагностический текст в виде ограниченной строки.
//
// @returns:
//   - result1: Ограниченный буферизованный диагностический вывод.
func (b *logTail) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.data)
}
