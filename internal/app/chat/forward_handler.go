package chatapp

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/meet-space/internal/app/httpresponse"
	"github.com/janickiy/meet-space/internal/domain/chat"
	httpmiddleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
)

func (h *Handler) Forward(c *gin.Context) {
	id, ok := parameter(c, "id")
	if !ok {
		return
	}
	var request chat.ForwardRequest
	if !httpresponse.BindJSON(c, &request, false) {
		return
	}
	items, created, err := h.service.Forward(c.Request.Context(), httpmiddleware.UserID(c), id, request)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	c.JSON(status, gin.H{"status": "success", "items": items})
}
