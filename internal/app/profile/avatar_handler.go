package profileapp

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/meet-space/internal/app/httpresponse"
	"github.com/janickiy/meet-space/internal/domain/avatar"
	"github.com/janickiy/meet-space/internal/transport/http/middleware"
	"github.com/janickiy/meet-space/internal/usecase/profile"
)

type AvatarHandler struct{ Service *profile.AvatarService }

// Put reads a bounded binary image for the authenticated user, not a body user ID.
func (h *AvatarHandler) Put(c *gin.Context) {
	controller := http.NewResponseController(c.Writer)
	_ = controller.SetReadDeadline(time.Now().Add(20 * time.Second))
	defer controller.SetReadDeadline(time.Time{})
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, avatar.MaxUploadBytes+1)
	user, err := h.Service.Put(c.Request.Context(), httpmiddleware.UserID(c), c.GetHeader("Content-Type"), c.Request.Body)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "user": user})
}

// Delete removes only the current user's avatar.
func (h *AvatarHandler) Delete(c *gin.Context) {
	user, err := h.Service.Delete(c.Request.Context(), httpmiddleware.UserID(c))
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "user": user})
}

// Content serves a normalized JPEG to authenticated directory users without caching.
func (h *AvatarHandler) Content(c *gin.Context) {
	data, err := h.Service.Get(c.Request.Context(), httpmiddleware.UserID(c), c.Param("id"))
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.Data(http.StatusOK, "image/jpeg", data)
}
