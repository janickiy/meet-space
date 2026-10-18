package media

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/config"
	domain "github.com/janickiy/meet-space/internal/domain/media"
)

// TestControllerClampsConfiguredSDPAtProtocolLimit verifies controller clamps configured SDP at
// protocol limit to prevent regressions in authenticated media-room control.
//
// Parameters:
//   - t: Test handle used for assertions, cleanup and failure reporting.
func TestControllerClampsConfiguredSDPAtProtocolLimit(t *testing.T) {
	c := NewController(nil, nil, nil, nil, nil, config.MediaConfig{}, config.RealtimeConfig{SDPBytes: 2 * domain.MaxSDPBytes, ICEBytes: domain.MaxICEBytes})
	for _, size := range []int{49153, 65536, domain.MaxSDPBytes, domain.MaxSDPBytes + 1} {
		raw, _ := json.Marshal(map[string]any{"mediaPeerId": uuid.NewString(), "negotiationId": uuid.NewString(), "sdp": strings.Repeat("x", size), "publications": []any{}})
		_, err := c.decode("media.offer", raw)
		if size <= domain.MaxSDPBytes && err != nil {
			t.Fatalf("in-range offer %d rejected: %v", size, err)
		}
		if size > domain.MaxSDPBytes && !errors.Is(err, domain.ErrInvalid) {
			t.Fatalf("oversized SDP %d accepted: %v", size, err)
		}
	}
}
