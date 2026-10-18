// Package platformapp exposes safe product capabilities and administrator-only operational
// views.
package platformapp

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/meet-space/internal/app/httpresponse"
	domain "github.com/janickiy/meet-space/internal/domain/platform"
)

type Service interface {
	// Capabilities returns the product or integration capabilities actually enabled by the operator.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//
	// Returns:
	//   - result 1: The resulting capabilities.
	Capabilities(context.Context) domain.Capabilities
	// Summary returns bounded administrator-only operational aggregates and safe failure codes
	// without user content.
	//
	// Parameters:
	//   - argument 1: Context controlling cancellation, deadlines and operation lifetime.
	//
	// Returns:
	//   - result 1: The resulting summary.
	//   - result 2: nil on success; otherwise the validation, access, transport or persistence
	//     failure.
	Summary(context.Context) (domain.Summary, error)
}

type Handler struct {
	Service      Service
	BuildVersion string
}

// Capabilities returns the product or integration capabilities actually enabled by the operator.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) Capabilities(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "success", "capabilities": h.Service.Capabilities(c.Request.Context()), "buildVersion": h.BuildVersion})
}

// Summary returns bounded administrator-only operational aggregates and safe failure codes
// without user content.
//
// Parameters:
//   - c: Gin request and response context, including the authenticated identity.
func (h *Handler) Summary(c *gin.Context) {
	value, err := h.Service.Summary(c.Request.Context())
	if err != nil {
		httpresponse.Fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "item": value})
}
