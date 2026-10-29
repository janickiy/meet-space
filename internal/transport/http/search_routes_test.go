package httptransport

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	searchapp "github.com/janickiy/meet-space/internal/app/search"
	domain "github.com/janickiy/meet-space/internal/domain/search"
)

type keywordSearchStub struct{ calls int }

func (s *keywordSearchStub) Search(context.Context, string, domain.Query) (domain.Page, error) {
	s.calls++
	return domain.Page{EffectiveMode: "keyword", Items: []domain.Result{}, Limit: 20}, nil
}

func TestSearchRoutePreservesPrivateResponseAndRemovedRoutesAreAbsent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	service := &keywordSearchStub{}
	RegisterSearchRoutes(router, &searchapp.Handler{Service: service}, func(c *gin.Context) { c.Next() })
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, APIV1Prefix+"/search?q=meeting", nil))
	if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "private, no-store" || service.calls != 1 {
		t.Fatalf("search status=%d headers=%v calls=%d", response.Code, response.Header(), service.calls)
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if _, exists := body["fallbackReason"]; exists {
		t.Fatal("removed semantic fallback field remains")
	}
	for _, path := range []string{
		"/conferences/id/captions", "/conferences/id/captions/segments", "/conferences/id/analytics",
		"/conferences/id/recordings/id/search/reindex", "/conferences/id/recordings/id/transcript",
		"/conferences/id/recordings/id/transcript/segments", "/conferences/id/recordings/id/transcript/retry",
		"/conferences/id/recordings/id/summary", "/conferences/id/recordings/id/summary/regenerate",
	} {
		for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPut} {
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(method, APIV1Prefix+path, nil))
			if response.Code != http.StatusNotFound {
				t.Errorf("%s %s returned %d", method, path, response.Code)
			}
		}
	}
}

func TestSearchRouteRequiresAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	service := &keywordSearchStub{}
	RegisterSearchRoutes(router, &searchapp.Handler{Service: service}, func(c *gin.Context) { c.AbortWithStatus(http.StatusUnauthorized) })
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, APIV1Prefix+"/search?q=meeting", nil))
	if response.Code != http.StatusUnauthorized || service.calls != 0 {
		t.Fatalf("status=%d calls=%d", response.Code, service.calls)
	}
}
