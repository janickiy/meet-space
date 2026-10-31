package recordingsapp

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/meet-space/internal/app/httpresponse"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/records"
	httpmiddleware "github.com/janickiy/meet-space/internal/transport/http/middleware"
)

// Library returns all readable recordings independently of the meeting timeline.
func (h *Handler) Library(c *gin.Context) {
	query := records.LibraryQuery{Cursor: c.Query("cursor"), Limit: 30}
	parameters := c.Request.URL.Query()
	if values, exists := parameters["cursor"]; exists && len(values) != 1 {
		httpresponse.Fail(c, apperrors.ErrInvalidInput)
		return
	}
	if values, exists := parameters["limit"]; exists {
		if len(values) != 1 {
			httpresponse.Fail(c, apperrors.ErrInvalidInput)
			return
		}
		limit, err := strconv.Atoi(values[0])
		if err != nil {
			httpresponse.Fail(c, apperrors.ErrInvalidInput)
			return
		}
		query.Limit = limit
	}
	if err := query.Validate(); err != nil {
		httpresponse.Fail(c, err)
		return
	}
	service, ok := h.service.(interface {
		Library(context.Context, string, records.LibraryQuery) (records.LibraryPage, error)
	})
	if !ok {
		httpresponse.Fail(c, apperrors.ErrUnavailable)
		return
	}
	page, err := service.Library(c.Request.Context(), httpmiddleware.UserID(c), query)
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "items": page.Items, "nextCursor": page.NextCursor})
}
