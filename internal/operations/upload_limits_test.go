package operations

import (
	"bytes"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/meet-space/internal/config"
)

// TestBinaryUploadLimits keeps the JSON cap everywhere except exact PUT upload
// routes. Their own handlers remain responsible for bounded binary decoding.
func TestBinaryUploadLimits(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(New("api", "upload-test", config.OperationsConfig{HTTPBodyBytes: 1 << 20}, nil).Middleware())
	for _, route := range []string{
		"/api/v1/auth/me/avatar", "/api/v1/conversations/:id/avatar",
		"/api/v1/conferences/:id/attachments/:attachmentId/content",
		"/api/v1/conversations/:id/attachments/:attachmentId/content",
		"/api/v1/auth/me/avatar/:extra", "/api/v1/conversations/:id/avatar/content",
		"/api/v1/auth/login",
	} {
		router.Handle("PUT", route, func(c *gin.Context) {
			if _, err := io.Copy(io.Discard, c.Request.Body); err != nil {
				c.Status(413)
				return
			}
			c.Status(204)
		})
	}
	router.POST("/api/v1/auth/me/avatar", func(c *gin.Context) {
		if _, err := io.Copy(io.Discard, c.Request.Body); err != nil {
			c.Status(413)
			return
		}
		c.Status(204)
	})
	for _, tc := range []struct {
		method, path string
		want         int
	}{
		{"PUT", "/api/v1/auth/me/avatar", 204},
		{"PUT", "/api/v1/conversations/group/avatar", 204},
		{"PUT", "/api/v1/conferences/meeting/attachments/file/content", 204},
		{"PUT", "/api/v1/conversations/group/attachments/file/content", 204},
		{"PUT", "/api/v1/auth/me/avatar/extra", 413},
		{"PUT", "/api/v1/conversations/group/avatar/content", 413},
		{"PUT", "/api/v1/auth/login", 413},
		{"POST", "/api/v1/auth/me/avatar", 413},
	} {
		for _, chunked := range []bool{false, true} {
			req := httptest.NewRequest(tc.method, tc.path, bytes.NewReader(make([]byte, 3<<19)))
			if chunked {
				req.ContentLength = -1
			}
			out := httptest.NewRecorder()
			router.ServeHTTP(out, req)
			if out.Code != tc.want {
				t.Fatalf("%s %s chunked=%v: code=%d want=%d", tc.method, tc.path, chunked, out.Code, tc.want)
			}
		}
	}
}
