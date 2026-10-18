package captions

import (
	"encoding/binary"
	"math"
)

// AudioActive classifies whether a PCM audio frame contains activity above the configured
// detection threshold.
//
// Parameters:
//   - pcm: Input bytes consumed by this operation.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
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
