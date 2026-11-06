package httptransport

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/meet-space/internal/app/httpresponse"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/chat"
	"github.com/janickiy/meet-space/internal/domain/settings"
)

// RegisterAttachmentPolicyRoutes exposes only the current size limit to signed-in
// users and conference guests. Upload endpoints remain authoritative.
func RegisterAttachmentPolicyRoutes(router gin.IRouter, reader settings.Reader, authentication gin.HandlerFunc) {
	private := func(c *gin.Context) {
		c.Header("Cache-Control", "private, no-store")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Next()
	}
	router.GET(APIV1Prefix+"/attachments/policy", private, authentication, func(c *gin.Context) {
		if reader == nil {
			httpresponse.Fail(c, apperrors.ErrUnavailable)
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()
		values, err := reader.Snapshot(ctx)
		if err != nil || values.AttachmentMaxBytes < 1024 || values.AttachmentMaxBytes > chat.AbsoluteMaxAttachmentBytes {
			httpresponse.Fail(c, apperrors.ErrUnavailable)
			return
		}
		c.JSON(200, gin.H{"maxBytes": values.AttachmentMaxBytes})
	})
}
