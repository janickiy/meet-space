// MeetSpace
// Copyright 2026 Alexander Yanitskiy
// Author: Alexander Yanitskiy
// Email: janickiy@mail.ru
// Telegram: @janickiy (https://t.me/janickiy)
// Website: https://janickiy.com/
// Licensed under the Apache License, Version 2.0.

// Пакет operations обеспечивает безопасную проверку готовности процессов и предоставляет
// соответствующие метрики.
package operations

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/pprof"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/janickiy/meet-space/internal/buildinfo"
	"github.com/janickiy/meet-space/internal/config"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Check — проверяет одну критическую зависимость в рамках предоставленного ограниченного контекста. nil
// означает готовность; текст с сообщением об ошибке никогда не отображается в ответах о
// работоспособности.
type Check func(context.Context) error

// Runtime — среда выполнения отвечает за режим подготовки к остановке, кэшированную готовность и изолированный реестр
// метрик.  Зависимости проверяются по таймеру, а не при каждом запросе HTTP.  Секрет защищает
// только приватные метрики.  Счетчики используют ограниченные технические метки.
type Runtime struct {
	beginDrain      func()
	activeWork      func() int
	inflight        atomic.Int64
	Config          config.OperationsConfig
	Checks          map[string]Check
	Registry        *prometheus.Registry
	ready, draining atomic.Bool
	dependency      *prometheus.GaugeVec
	dependencyMu    sync.RWMutex
	dependencyState map[string]bool
	requests        *prometheus.CounterVec
	duration        *prometheus.HistogramVec
	ws              prometheus.Gauge
	wsMessages      *prometheus.CounterVec
	custom          *prometheus.GaugeVec
	customMu        sync.Mutex
	customNames     map[string]struct{}
	events          *prometheus.CounterVec
	workDuration    *prometheus.HistogramVec
	productJobs     *prometheus.CounterVec
	productDuration *prometheus.HistogramVec
	productQueue    *prometheus.GaugeVec
	providerCalls   *prometheus.CounterVec
	providerTime    *prometheus.HistogramVec
	searchRequests  *prometheus.CounterVec
	searchTime      prometheus.Histogram
}

var current atomic.Pointer[Runtime]

type correlationKey struct{}

// New создает компонент с зависимостями и ограничениями обработки, необходимыми для проверки
// готовности процесса, использования ограниченных метрик и корректного завершения работы.
//
// @Parameters:
//   - service: Фиксированное имя сервиса, используемое для ограниченных операционных меток.
//   - instance: Метка экземпляра процесса, контролируемая оператором.
//   - cfg: Ограничения конфигурации и обработки, контролируемые оператором.
//   - checks: Именованные проверки готовности зависимостей или маршрутные проверки.
//
// @returns:
//   - result1: Созданная или загруженная среда выполнения.
func New(service, instance string, cfg config.OperationsConfig, checks map[string]Check) *Runtime {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("service", service, "instance_id", instance))
	r := &Runtime{Config: cfg, Checks: checks, Registry: prometheus.NewRegistry(), customNames: map[string]struct{}{}, dependencyState: map[string]bool{}}
	build := buildinfo.Current()
	buildMetric := prometheus.NewGauge(prometheus.GaugeOpts{Name: "recorder_build_info", Help: "Immutable public release metadata.", ConstLabels: prometheus.Labels{"version": build.Version, "commit": build.Commit}})
	buildMetric.Set(1)
	r.Registry.MustRegister(buildMetric)
	r.dependency = prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "recorder_dependency_up", Help: "Last bounded dependency probe: 1 healthy, 0 unavailable."}, []string{"dependency"})
	r.requests = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "recorder_http_requests_total", Help: "Completed HTTP requests by route template, method and status."}, []string{"route", "method", "status"})
	r.duration = prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "recorder_http_duration_seconds", Help: "HTTP handler duration; SSE includes stream lifetime, WS counts handshake only.", Buckets: []float64{.005, .025, .1, .25, .5, 1, 2, 5, 10}}, []string{"route", "method"})
	w := prometheus.NewGauge(prometheus.GaugeOpts{Name: "recorder_ws_active", Help: "Physical WebSocket connections on this API."})
	r.ws = w
	r.wsMessages = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "recorder_ws_messages_total", Help: "Received WebSocket messages by bounded class."}, []string{"class"})
	r.custom = prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "recorder_state", Help: "Bounded application/dependency resource snapshots; see operations documentation."}, []string{"resource"})
	r.events = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "recorder_events_total", Help: "Bounded operational events and failures."}, []string{"event"})
	r.workDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "recorder_work_duration_seconds", Help: "Command, recording and finalization duration.", Buckets: []float64{.01, .1, 1, 5, 15, 60, 300, 1800, 7200}}, []string{"operation"})
	r.Registry.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}), r.dependency, r.requests, r.duration, w, r.wsMessages, r.custom, r.events, r.workDuration)
	r.productJobs = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "recorder_product_jobs_total", Help: "Product job attempts by fixed kind and safe outcome."}, []string{"kind", "outcome"})
	r.productDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "recorder_product_job_duration_seconds", Help: "Off-hot-path provider job duration.", Buckets: []float64{.01, .1, 1, 5, 15, 60, 300, 900}}, []string{"kind"})
	r.productQueue = prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "recorder_product_queue", Help: "Persistent product backlog by fixed kind and state."}, []string{"kind", "state"})
	r.providerCalls = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "recorder_product_provider_calls_total", Help: "Provider usage attempts without payload, URL, model or identity labels."}, []string{"provider", "outcome"})
	r.providerTime = prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "recorder_product_provider_duration_seconds", Help: "Individual provider call latency.", Buckets: []float64{.01, .1, 1, 5, 15, 60, 300, 900}}, []string{"provider"})
	r.searchRequests = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "recorder_search_requests_total", Help: "Search requests by safe result class, never query text."}, []string{"outcome"})
	r.searchTime = prometheus.NewHistogram(prometheus.HistogramOpts{Name: "recorder_search_duration_seconds", Help: "Permission-filtered search HTTP latency.", Buckets: []float64{.005, .025, .1, .25, .5, 1, 2, 5}})
	r.Registry.MustRegister(r.productJobs, r.productDuration, r.productQueue, r.providerCalls, r.providerTime, r.searchRequests, r.searchTime)
	current.Store(r)
	return r
}

// Run выполняет проверку готовности процесса, использование ограниченных метрик и корректное
// завершение работы до тех пор, пока контекст не будет отменен или не произойдет сбой
// обработки.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//
// @returns:
//   - result1: Ограниченное событие или медиаканал, принадлежащий этой операции.
func (r *Runtime) Run(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})
	r.probe(ctx)
	go func() {
		defer close(done)
		ticker := time.NewTicker(r.Config.ProbeInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				r.Drain()
				return
			case <-ticker.C:
				r.probe(ctx)
			}
		}
	}()
	return done
}

// probe — проверка обновляет результаты проверки готовности ограниченных зависимостей для последующих
// запросов о работоспособности.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
func (r *Runtime) probe(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, r.Config.ProbeTimeout)
	defer cancel()
	ok := true
	states := make(map[string]bool, len(r.Checks))
	for name, check := range r.Checks {
		value := 1.
		healthy := check(ctx) == nil
		if !healthy {
			Event("dependency_failed")
			value = 0
			ok = false
		}
		states[name] = healthy
		r.dependency.WithLabelValues(name).Set(value)
	}
	r.dependencyMu.Lock()
	r.dependencyState = states
	r.dependencyMu.Unlock()
	r.ready.Store(ok && !r.draining.Load())
}

// Drain отклоняет новые задачи и предоставляет информацию о состоянии активных задач,
// используемую при корректном развертывании.
func (r *Runtime) Drain() { r.draining.Store(true); r.ready.Store(false) }

// Ready сообщает, можно ли в данный момент использовать компонент или настроенную зависимость.
//
// @returns:
//   - result1: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
func (r *Runtime) Ready() bool { return r.ready.Load() && !r.draining.Load() }

// DependencyStatuses возвращает моментальный снимок последних проверенных состояний
// зависимостей.
//
// @returns:
//   - result1: Результирующая карта [строка]bool.
func (r *Runtime) DependencyStatuses() map[string]bool {
	r.dependencyMu.RLock()
	defer r.dependencyMu.RUnlock()
	states := make(map[string]bool, len(r.dependencyState))
	for name, healthy := range r.dependencyState {
		states[name] = healthy
	}
	return states
}

// Live отвечает за работоспособность процесса, не проверяя внешние зависимости.
//
// @Parameters:
//   - w: Ответ HTTP для назначения.
//   - req: Входящий запрос HTTP.
func (r *Runtime) Live(w http.ResponseWriter, req *http.Request) { health(w, http.StatusOK, "alive") }

// Readiness отвечает за состояние готовности кэшированных данных и обеспечивает корректное
// завершение работы.
//
// @Parameters:
//   - w: Ответ HTTP для назначения.
//   - req: Входящий запрос HTTP.
func (r *Runtime) Readiness(w http.ResponseWriter, req *http.Request) {
	if r.Ready() {
		health(w, 200, "ready")
	} else {
		health(w, 503, "not_ready")
	}
}

// health — отвечает за работоспособность, используя выбранные проверки готовности или работоспособности.
//
// @Parameters:
//   - w: Ответ HTTP для назначения.
//   - status: Состояние жизненного цикла или статус ответа на запись.
//   - state: Текущее состояние или обновлённое валидированное состояние.
func health(w http.ResponseWriter, status int, state string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": state})
}

// Authorized проверяет наличие авторизации на диагностику, предоставленной оператором, перед
// отправкой защищенных операционных данных.
//
// @Parameters:
//   - req: Входящий запрос HTTP.
//   - secret: Материал для создания закрытого ключа или необработанный секретный ключ; его
//     нельзя регистрировать или раскрывать.
//
// @returns:
//   - result1: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
func Authorized(req *http.Request, secret string) bool {
	return len(secret) >= 32 && subtle.ConstantTimeCompare([]byte(req.Header.Get("Authorization")), []byte("Bearer "+secret)) == 1
}

// Metrics отвечает за метрики Prometheus, не используя идентификаторы сущностей в качестве
// меток.
//
// @Parameters:
//   - w: Ответ HTTP для назначения.
//   - req: Входящий запрос HTTP.
func (r *Runtime) Metrics(w http.ResponseWriter, req *http.Request) {
	if !Authorized(req, r.Config.MetricsSecret) {
		http.NotFound(w, req)
		return
	}
	promhttp.HandlerFor(r.Registry, promhttp.HandlerOpts{Timeout: 5 * time.Second, MaxRequestsInFlight: 2}).ServeHTTP(w, req)
}

// Register добавляет конечные точки работоспособности, готовности и защищенных метрик во
// внутренний мультиплексор сервисов.
//
// @Parameters:
//   - mux: Мультиплексор запросов HTTP для получения зарегистрированных маршрутов.
func (r *Runtime) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /version", buildinfo.Handler)
	mux.HandleFunc("GET /operations/drain", r.DrainStatus)
	mux.HandleFunc("POST /operations/drain", r.DrainStatus)
	mux.HandleFunc("GET /health/live", r.Live)
	mux.HandleFunc("GET /health/ready", r.Readiness)
	mux.HandleFunc("GET /metrics", r.Metrics)
}

// RegisterGin регистрирует конечные точки работоспособности, готовности, метрик и завершения
// работы в маршрутизаторе Gin.
//
// @Parameters:
//   - router: Маршрутизатор HTTP, в котором зарегистрированы конечные точки компонентов.
func (r *Runtime) RegisterGin(router *gin.Engine) {
	router.GET("/version", gin.WrapF(buildinfo.Handler))
	router.GET("/operations/drain", gin.WrapF(r.DrainStatus))
	router.POST("/operations/drain", gin.WrapF(r.DrainStatus))
	router.GET("/health/live", gin.WrapF(r.Live))
	router.GET("/health/ready", gin.WrapF(r.Readiness))
	router.GET("/metrics", gin.WrapF(r.Metrics))
}

// Middleware — промежуточное ПО измеряет активность HTTP, используя шаблоны ограниченных маршрутов, а не
// приватные пути запросов.
//
// @returns:
//   - result1: Обработчик HTTP для регистрации этой операции.
func (r *Runtime) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		id := c.Request.Header.Get("X-Request-ID")
		if _, err := uuid.Parse(id); err != nil {
			id = uuid.NewString()
		}
		c.Header("X-Request-ID", id)
		c.Request = c.Request.WithContext(WithID(c.Request.Context(), id))
		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		if route != "/operations/drain" && route != "/metrics" && route != "/version" && route != "/health/live" && route != "/health/ready" {
			r.inflight.Add(1)
			defer r.inflight.Add(-1)
		}
		defer func() {
			method := c.Request.Method
			switch method {
			case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS":
			default:
				method = "other"
			}
			r.requests.WithLabelValues(route, method, strconv.Itoa(c.Writer.Status())).Inc()
			r.duration.WithLabelValues(route, method).Observe(time.Since(start).Seconds())
		}()
		if r.draining.Load() && route != "/health/live" && route != "/health/ready" && route != "/metrics" && route != "/version" && route != "/operations/drain" {
			c.AbortWithStatus(503)
			return
		}
		if c.Request.Body != nil && !(c.Request.Method == http.MethodPut && stringsHasUploadSuffix(route)) {
			if c.Request.ContentLength > r.Config.HTTPBodyBytes {
				c.AbortWithStatus(413)
				return
			}
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, r.Config.HTTPBodyBytes)
		}
		c.Next()
	}
}

// stringsHasUploadSuffix распознает суффикс маршрута загрузки, не допуская использования
// произвольных вариантов пути.
//
// @Parameters:
//   - route: Текущий владелец медиа-комнаты и версия ограждения.
//
// @returns:
//   - result1: Указывает, выполняется ли задокументированное условие операции и произошло
//     ли успешное изменение ее условного состояния.
func stringsHasUploadSuffix(route string) bool {
	return route == "/api/v1/conferences/:id/attachments/:attachmentId/content" ||
		route == "/api/v1/conversations/:id/attachments/:attachmentId/content" ||
		route == "/api/v1/conversations/:id/avatar" ||
		route == "/api/v1/auth/me/avatar"
}

// WithID возвращает контекст, содержащий идентификатор корреляции запроса или операции.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//   - id: Идентификатор ресурса, к которому обращается данная операция.
//
// @returns:
//   - result1: Производный контекст, содержащий информацию о состоянии операции или времени
//     ее отмены.
func WithID(ctx context.Context, id string) context.Context {
	if _, err := uuid.Parse(id); err != nil {
		return ctx
	}
	return context.WithValue(ctx, correlationKey{}, id)
}

// ID считывает идентификатор корреляции, связанный с текущим контекстом операции.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//
// @returns:
//   - result1: Идентификатор корреляции из контекста операции.
func ID(ctx context.Context) string { id, _ := ctx.Value(correlationKey{}).(string); return id }

// State обновляет фиксированный индикатор технического ресурса без создания временных рядов,
// привязанных к конкретному объекту.
//
// @Parameters:
//   - name: Имя параметра, файла cookie, ресурса или провайдера, к которому осуществляется
//     обращение.
//   - value: Значение для проверки, нормализации или преобразования.
func State(name string, value float64) {
	if r := current.Load(); r != nil {
		if len(name) > 64 {
			return
		}
		r.customMu.Lock()
		_, known := r.customNames[name]
		if !known && len(r.customNames) >= 64 {
			r.customMu.Unlock()
			return
		}
		r.customNames[name] = struct{}{}
		r.customMu.Unlock()
		r.custom.WithLabelValues(name).Set(value)
	}
}

// WSActive применяет дельту подключения или отключения к индикатору active-WebSocket.
//
// @Parameters:
//   - delta: Положительная или отрицательная корректировка количества событий в течение
//     жизненного цикла.
func WSActive(delta float64) {
	if r := current.Load(); r != nil {
		r.ws.Add(delta)
	}
}

// Event — событие увеличивает один из фиксированных счетчиков операционных событий.
//
// @Parameters:
//   - name: Имя параметра, файла cookie, ресурса или провайдера, к которому осуществляется
//     обращение.
func Event(name string) {
	switch name {
	case "ffmpeg_failed", "recording_failed", "recording_ready", "rabbitmq_dead_letter", "rabbitmq_reconnect", "dependency_failed", "disk_low", "live_reconnect", "live_capacity", "live_audio_dropped", "live_provider_failed", "live_decoder_failed", "live_invalid_event", "live_caption_limit", "live_persist_failed", "live_delivery_failed", "analytics_failed", "caption_partial", "caption_final", "recording_mode_composite", "recording_mode_audio_only", "recording_mode_individual_tracks", "recording_mode_screen_focus":
	default:
		return
	}
	if r := current.Load(); r != nil {
		r.events.WithLabelValues(name).Inc()
	}
}

// Observe записывает измеренную продолжительность фиксированной разрешенной операции без
// привязки к конкретным метрикам объекта.
//
// @Parameters:
//   - name: Имя параметра, файла cookie, ресурса или провайдера, к которому осуществляется
//     обращение.
//   - seconds: Измеренная продолжительность операции в секундах.
func Observe(name string, seconds float64) {
	switch name {
	case "recording", "finalization", "command", "ffmpeg", "search", "transcription", "summary", "email", "push", "calendar", "caption_latency", "caption_partial", "caption_final", "live_decode", "live_decode_cpu", "analytics", "embedding", "hybrid_search":
	default:
		return
	}
	if r := current.Load(); r != nil {
		r.workDuration.WithLabelValues(name).Observe(seconds)
	}
}

// FFmpegActive применяет дельта-значение для запуска или остановки к датчику активного процесса
// FFmpeg.
//
// @Parameters:
//   - delta: Положительная или отрицательная корректировка количества событий в течение
//     жизненного цикла.
func FFmpegActive(delta float64) {
	if r := current.Load(); r != nil {
		r.custom.WithLabelValues("ffmpeg_active").Add(delta)
	}
}

// WSMessage подсчитывает количество сообщений в реальном времени, используя фиксированный набор
// меток протокола.
//
// @Parameters:
//   - kind: Подтвержденное событие, задание, категория медиаданных или ресурсов.
func WSMessage(kind string) {
	class := "other"
	switch kind {
	case "media.join", "media.offer", "media.answer", "media.ice", "media.ready", "media.leave":
		class = "media"
	case "signal.offer", "signal.answer", "signal.ice":
		class = "signal"
	case "ping", "pong":
		class = "heartbeat"
	}
	if r := current.Load(); r != nil {
		r.wsMessages.WithLabelValues(class).Inc()
	}
}

// Profiling запускает конечную точку профилирования только при обратной связи, если она явно
// включена оператором.
//
// @Parameters:
//   - ctx: Контекст, контролирующий отмену, сроки и продолжительность выполнения операции.
//
// @returns:
//   - result1: nil — в случае успеха; в противном случае — ошибка валидации, доступа,
//     передачи данных или сохранения.
func (r *Runtime) Profiling(ctx context.Context) error {
	if r.Config.PprofPort == 0 {
		return nil
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	mux.Handle("/debug/pprof/heap", pprof.Handler("heap"))
	mux.Handle("/debug/pprof/goroutine", pprof.Handler("goroutine"))
	mux.Handle("/debug/pprof/mutex", pprof.Handler("mutex"))
	mux.Handle("/debug/pprof/block", pprof.Handler("block"))
	server := &http.Server{Addr: "127.0.0.1:" + strconv.Itoa(r.Config.PprofPort), Handler: boundedProfile(mux), ReadHeaderTimeout: 5 * time.Second, WriteTimeout: 65 * time.Second, IdleTimeout: 10 * time.Second}
	// Отменить регистрацию при ошибке привязки или нормальном возврате; не сохранять объект
	// ожидания до конца жизненного цикла процесса, если сервер так и не запустился.
	stop := context.AfterFunc(ctx, func() { _ = server.Close() })
	defer stop()
	return server.ListenAndServe()
}

// boundedProfile обрабатывает запрос на профилирование с ограниченной продолжительностью и
// контролируемым доступом к диагностике.
//
// @Parameters:
//   - next: Следующий обработчик HTTP или более новое событие, в зависимости от его типа.
//
// @returns:
//   - result1: Результирующий обработчик.
func boundedProfile(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if value := req.URL.Query().Get("seconds"); value != "" {
			seconds, err := strconv.ParseFloat(value, 64)
			if err != nil || !(seconds > 0 && seconds <= 60) {
				http.Error(w, "seconds must be greater than 0 and at most 60", http.StatusBadRequest)
				return
			}
		}
		next.ServeHTTP(w, req)
	})
}
