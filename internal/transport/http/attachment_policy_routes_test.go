package httptransport

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/meet-space/internal/domain/settings"
)

type attachmentPolicyReader struct {
	max   int64
	err   error
	calls int
}

func (r *attachmentPolicyReader) Snapshot(ctx context.Context) (settings.Values, error) {
	r.calls++
	if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > 3*time.Second {
		return settings.Values{}, errors.New("unbounded policy read")
	}
	return settings.Values{AttachmentMaxBytes: r.max, SMTPHost: "private-not-in-policy"}, r.err
}
func TestAttachmentPolicyRequiresAuthAndReturnsCurrentLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	reader := &attachmentPolicyReader{max: 1 << 20}
	router := gin.New()
	RegisterAttachmentPolicyRoutes(router, reader, func(c *gin.Context) {
		if c.GetHeader("Authorization") == "" {
			c.AbortWithStatus(401)
		}
	})
	get := func(auth bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest("GET", APIV1Prefix+"/attachments/policy", nil)
		if auth {
			req.Header.Set("Authorization", "synthetic")
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}
	if rec := get(false); rec.Code != 401 || reader.calls != 0 || rec.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatal("policy allowed unauthenticated read")
	}
	for _, limit := range []int64{1 << 20, 20 << 20} {
		reader.max = limit
		rec := get(true)
		var values map[string]int64
		if err := json.Unmarshal(rec.Body.Bytes(), &values); err != nil || rec.Code != 200 || len(values) != 1 || values["maxBytes"] != limit {
			t.Fatal("policy did not return current size only", err)
		}
	}
	reader.err = errors.New("private-database-error")
	if rec := get(true); rec.Code != 503 || strings.Contains(rec.Body.String(), "private") {
		t.Fatal("database failure did not fail safely")
	}
	reader.err = nil
	reader.max = 0
	if rec := get(true); rec.Code != 503 {
		t.Fatal("invalid policy was exposed")
	}
}
