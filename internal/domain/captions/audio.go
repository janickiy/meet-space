package captions

import (
	"encoding/binary"
	"math"
)

// AudioActive определяет, содержит ли аудиокадр PCM активность, превышающую заданный порог обнаружения
// .
//
// @Parameters:
// - pcm: входные байты, потребляемые этой операцией.
//
// @returns:
// - result1: Выполняется ли задокументированное условие операции или ее условное состояние.
// изменение выполнено успешно.
func AudioActive(pcm []byte) bool {
	var sum float64
	n := len(pcm) / 2
	if n == 0 {
		return false
	}
	for i := 0; i+1 < len(pcm); i += 2 {
		v := float64(int16(binary.LittleEndian.Uint16(pcm[i:i+2]))) / 32768
		sum += v * v
	}
	return math.Sqrt(sum/float64(n)) >= 0.007943282
}
