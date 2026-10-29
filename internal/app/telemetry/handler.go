// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

// Пакет телеметрии принимает очищенные от конфиденциальных данных отчеты о технических ошибках
// браузера.
package telemetry

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"regexp"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
)

const maxBodyBytes = 8192

var versionPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]{0,79}$`)
var assetPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+-[A-Za-z0-9_-]{8,}\.js$`)

// Frame содержит только позицию в связанном ресурсе, а не URL, имя функции или сообщение об
// ошибке.  Файл — это имя ресурса; строка и столбец — это положительные позиции.
type Frame struct {
	File   string `json:"file"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}

// Report содержит только разрешенные диагностические поля: версия фронтенда, шаблон маршрута,
// категория ошибки, семейство браузеров и не более пяти позиций связанных ресурсов.  Не
// содержит пользовательского контента.
type Report struct {
	Version string  `json:"version"`
	Route   string  `json:"route"`
	Code    string  `json:"code"`
	Browser string  `json:"browser"`
	Stack   []Frame `json:"stack"`
}

// Handler — обработчик принимает строго валидированные отчеты и подсчитывает количество сбоев с
// ограниченными метками.  Включен контроль приема данных; логгер получает очищенные от лишних
// данных поля; ошибки подсчитываются по фиксированным категориям.
type Handler struct {
	enabled bool
	logger  *slog.Logger
	errors  *prometheus.CounterVec
}

// NewHandler создает обработчик с зависимостями и ограничениями, необходимыми для очистки
// телеметрии об ошибках клиента.
//
// @Parameters:
//   - enabled: Включает или отключает запрошенную функцию или настройку.
//   - logger: Ограниченный диагностический логгер.
//   - registry: Реестр зависимостей или метрик, используемый для поиска или регистрации
//     компонента.
//
// @returns:
//   - result1: Сконструированный или загруженный обработчик.
func NewHandler(enabled bool, logger *slog.Logger, registry prometheus.Registerer) *Handler {
	counter := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "recorder_client_errors_total",
		Help: "Validated browser errors by fixed code, browser family and route template.",
	}, []string{"code", "browser", "route"})
	registry.MustRegister(counter)
	return &Handler{enabled: enabled, logger: logger, errors: counter}
}

// Receive проверяет и записывает ограниченные отчеты о технических ошибках клиента, не принимая
// конфиденциальные данные из браузера.
//
// @Parameters:
//   - c: Контекст запроса и ответа Gin, включая аутентифицированные идентификационные данные.
func (h *Handler) Receive(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	if !h.enabled {
		c.Status(http.StatusNotFound)
		return
	}
	mediaType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || mediaType != "application/json" {
		c.Status(http.StatusUnsupportedMediaType)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBodyBytes)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	var report Report
	if err := decoder.Decode(&report); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			c.Status(http.StatusRequestEntityTooLarge)
		} else {
			c.Status(http.StatusBadRequest)
		}
		return
	}
	if decoder.Decode(new(any)) != io.EOF || !validReport(report) {
		c.Status(http.StatusBadRequest)
		return
	}
	h.errors.WithLabelValues(report.Code, report.Browser, report.Route).Inc()
	h.logger.Warn("client error", "version", report.Version, "route", report.Route,
		"code", report.Code, "browser", report.Browser, "stack", report.Stack)
	c.Status(http.StatusAccepted)
}

// validReport проверяет схему телеметрии клиента и его ограниченные разрешенные поля.
//
// @Parameters:
//   - report: Валидированный отчет о технических ошибках клиента.
//
// @returns:
//   - result1: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
func validReport(report Report) bool {
	if !versionPattern.MatchString(report.Version) || len(report.Stack) > 5 {
		return false
	}
	switch report.Code {
	case "uncaught_error", "unhandled_rejection", "react_render_error":
	default:
		return false
	}
	switch report.Browser {
	case "chromium", "firefox", "safari", "edge", "other":
	default:
		return false
	}
	switch report.Route {
	case "/", "/login", "/register", "/register/success", "/app", "/meetings", "/meetings/new",
		"/meetings/:id", "/meetings/:id/join", "/conferences", "/conferences/new", "/conferences/:id",
		"/conferences/:id/join", "/i/:code", "/app/settings", "/settings",
		"/app/settings/calendar/:provider/callback", "/app/recordings", "/history", "/history/:id",
		"/notifications", "/app/search", "/search", "/admin", "/calendar", "unknown":
	default:
		return false
	}
	for _, frame := range report.Stack {
		if len(frame.File) > 160 || !assetPattern.MatchString(frame.File) || frame.Line < 1 || frame.Line > 10_000_000 || frame.Column < 1 || frame.Column > 10_000_000 {
			return false
		}
	}
	return true
}
