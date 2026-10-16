package httpresponse

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/meet-space/internal/domain/apperrors"
)

// InternalErrorMessage is the safe public message for unexpected server errors.
const InternalErrorMessage = "internal server error"

// Fail сопоставляет прикладную ошибку с HTTP-статусом и безопасным JSON-ответом.
// Для Internal всегда используется общий текст без диагностической причины.
func Fail(c *gin.Context, err error) {
	status, message := http.StatusInternalServerError, InternalErrorMessage
	switch apperrors.Classify(err) {
	case apperrors.Validation:
		status, message = http.StatusUnprocessableEntity, "invalid input"
	case apperrors.Unauthenticated:
		status, message = http.StatusUnauthorized, "authentication required or token invalid"
	case apperrors.Forbidden:
		status, message = http.StatusForbidden, "access denied"
	case apperrors.NotFound:
		status, message = http.StatusNotFound, "not found"
	case apperrors.Conflict:
		status, message = http.StatusConflict, "conflict"
	case apperrors.RateLimited:
		status, message = http.StatusTooManyRequests, "rate limit exceeded"
	case apperrors.Timeout:
		status, message = http.StatusGatewayTimeout, "operation timed out"
	case apperrors.Unavailable:
		status, message = http.StatusServiceUnavailable, "service temporarily unavailable"
	}
	var applicationError *apperrors.Error
	if status != http.StatusInternalServerError && errors.As(err, &applicationError) {
		message = applicationError.Message
	}
	c.AbortWithStatusJSON(status, gin.H{"status": "failed", "message": message})
}

// BindJSON декодирует одно JSON-значение в target с лимитом тела 32 КиБ,
// запрещая неизвестные поля и данные после значения. optional разрешает пустое
// тело. При false ответ 400 уже записан и обработка запроса прервана.
func BindJSON(c *gin.Context, target any, optional bool) bool {
	if c.Request.Body == nil {
		c.Request.Body = http.NoBody
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 32*1024)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	err := decoder.Decode(target)
	if optional && errors.Is(err, io.EOF) {
		return true
	}
	if err == nil {
		var trailing any
		if errors.Is(decoder.Decode(&trailing), io.EOF) {
			return true
		}
	}
	c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"status": "failed", "message": "invalid json body or unknown fields"})
	return false
}

// Pagination возвращает limit, offset и признак успешного разбора query-параметров.
// По умолчанию limit=20 и offset=0; допустимы limit от 1 до 100 и offset >= 0.
// При false ответ с ошибкой валидации уже записан.
func Pagination(c *gin.Context) (int, int, bool) {
	limit, offset := 20, 0
	var err error
	if raw, exists := c.GetQuery("limit"); exists {
		limit, err = strconv.Atoi(raw)
	}
	if err != nil || limit < 1 || limit > 100 {
		Fail(c, apperrors.New(apperrors.ErrInvalidInput, "limit must be between 1 and 100"))
		return 0, 0, false
	}
	if raw, exists := c.GetQuery("offset"); exists {
		offset, err = strconv.Atoi(raw)
	}
	if err != nil || offset < 0 {
		Fail(c, apperrors.New(apperrors.ErrInvalidInput, "offset must be non-negative"))
		return 0, 0, false
	}
	return limit, offset, true
}
