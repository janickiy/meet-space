// Package operations provides safe process readiness checks and metrics.
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

// Check probes one critical dependency within the supplied bounded context. nil means ready; the
// error text is never exposed in health responses.
type Check func(context.Context) error

// Runtime owns drain state, cached readiness and an isolated metrics registry. Dependencies are
// checked by a timer rather than every HTTP request. Secret protects only private metrics.
// Counters use bounded technical labels.
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

// New constructs the component with the dependencies and processing limits required for process
// readiness, bounded metrics and graceful shutdown.
//
// Parameters:
//   - service: Fixed service name used for bounded operational labels.
//   - instance: Operator-controlled process instance label.
//   - cfg: Operator-controlled configuration and processing limits.
//   - checks: Named dependency readiness probes or route checks.
//
// Returns:
//   - result 1: The constructed or loaded runtime.
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

// Run runs process readiness, bounded metrics and graceful shutdown until the context is
// canceled or processing fails.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//
// Returns:
//   - result 1: The bounded event or media channel owned by this operation.
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

// probe refreshes bounded dependency readiness results for subsequent health requests.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
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

// Drain rejects new work and exposes the active-work state used by graceful deployment.
func (r *Runtime) Drain() { r.draining.Store(true); r.ready.Store(false) }

// Ready reports whether the component or configured dependency is currently usable.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func (r *Runtime) Ready() bool { return r.ready.Load() && !r.draining.Load() }

// DependencyStatuses returns a snapshot of the most recently probed dependency states.
//
// Returns:
//   - result 1: The resulting map[string]bool.
func (r *Runtime) DependencyStatuses() map[string]bool {
	r.dependencyMu.RLock()
	defer r.dependencyMu.RUnlock()
	states := make(map[string]bool, len(r.dependencyState))
	for name, healthy := range r.dependencyState {
		states[name] = healthy
	}
	return states
}

// Live serves the process liveness response without probing external dependencies.
//
// Parameters:
//   - w: Destination HTTP response writer.
//   - req: Incoming HTTP request.
func (r *Runtime) Live(w http.ResponseWriter, req *http.Request) { health(w, http.StatusOK, "alive") }

// Readiness serves the cached readiness state and accounts for graceful draining.
//
// Parameters:
//   - w: Destination HTTP response writer.
//   - req: Incoming HTTP request.
func (r *Runtime) Readiness(w http.ResponseWriter, req *http.Request) {
	if r.Ready() {
		health(w, 200, "ready")
	} else {
		health(w, 503, "not_ready")
	}
}

// health writes the bounded health response using the selected readiness or liveness checks.
//
// Parameters:
//   - w: Destination HTTP response writer.
//   - status: Lifecycle state or response status to record.
//   - state: Current state or validated state update.
func health(w http.ResponseWriter, status int, state string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": state})
}

// Authorized checks the operator-controlled diagnostic authorization before serving protected
// operational data.
//
// Parameters:
//   - req: Incoming HTTP request.
//   - secret: Private key material or raw secret; it must not be logged or exposed.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func Authorized(req *http.Request, secret string) bool {
	return len(secret) >= 32 && subtle.ConstantTimeCompare([]byte(req.Header.Get("Authorization")), []byte("Bearer "+secret)) == 1
}

// Metrics serves Prometheus metrics without using entity identifiers as labels.
//
// Parameters:
//   - w: Destination HTTP response writer.
//   - req: Incoming HTTP request.
func (r *Runtime) Metrics(w http.ResponseWriter, req *http.Request) {
	if !Authorized(req, r.Config.MetricsSecret) {
		http.NotFound(w, req)
		return
	}
	promhttp.HandlerFor(r.Registry, promhttp.HandlerOpts{Timeout: 5 * time.Second, MaxRequestsInFlight: 2}).ServeHTTP(w, req)
}

// Register adds health, readiness and protected metrics endpoints to the internal service
// multiplexer.
//
// Parameters:
//   - mux: HTTP request multiplexer to receive the registered routes.
func (r *Runtime) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /version", buildinfo.Handler)
	mux.HandleFunc("GET /operations/drain", r.DrainStatus)
	mux.HandleFunc("POST /operations/drain", r.DrainStatus)
	mux.HandleFunc("GET /health/live", r.Live)
	mux.HandleFunc("GET /health/ready", r.Readiness)
	mux.HandleFunc("GET /metrics", r.Metrics)
}

// RegisterGin registers the operational health, readiness, metrics and drain endpoints on the
// Gin router.
//
// Parameters:
//   - router: HTTP router on which the component endpoints are registered.
func (r *Runtime) RegisterGin(router *gin.Engine) {
	router.GET("/version", gin.WrapF(buildinfo.Handler))
	router.GET("/operations/drain", gin.WrapF(r.DrainStatus))
	router.POST("/operations/drain", gin.WrapF(r.DrainStatus))
	router.GET("/health/live", gin.WrapF(r.Live))
	router.GET("/health/ready", gin.WrapF(r.Readiness))
	router.GET("/metrics", gin.WrapF(r.Metrics))
}

// Middleware measures HTTP activity using bounded route templates rather than private request
// paths.
//
// Returns:
//   - result 1: The HTTP handler to register for this operation.
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

// stringsHasUploadSuffix recognizes the upload route suffix without accepting arbitrary path
// variants.
//
// Parameters:
//   - route: Current media-room owner and fencing version.
//
// Returns:
//   - result 1: Whether the operation's documented condition holds or its conditional state
//     change succeeded.
func stringsHasUploadSuffix(route string) bool {
	return route == "/api/v1/conferences/:id/attachments/:attachmentId/content" ||
		route == "/api/v1/conversations/:id/attachments/:attachmentId/content"
}

// WithID returns a context carrying the request or operation correlation identifier.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//   - id: Identifier of the resource addressed by this operation.
//
// Returns:
//   - result 1: The derived context carrying the operation state or cancellation lifetime.
func WithID(ctx context.Context, id string) context.Context {
	if _, err := uuid.Parse(id); err != nil {
		return ctx
	}
	return context.WithValue(ctx, correlationKey{}, id)
}

// ID reads the correlation identifier associated with the current operation context.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//
// Returns:
//   - result 1: Correlation identifier from the operation context.
func ID(ctx context.Context) string { id, _ := ctx.Value(correlationKey{}).(string); return id }

// State updates a fixed technical resource gauge without introducing entity-specific time
// series.
//
// Parameters:
//   - name: Name of the setting, cookie, resource or provider being addressed.
//   - value: Value to validate, normalize or convert.
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

// WSActive applies a connection or disconnection delta to the active-WebSocket gauge.
//
// Parameters:
//   - delta: Positive or negative lifecycle count adjustment.
func WSActive(delta float64) {
	if r := current.Load(); r != nil {
		r.ws.Add(delta)
	}
}

// Event increments one of the fixed operational event counters.
//
// Parameters:
//   - name: Name of the setting, cookie, resource or provider being addressed.
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

// Observe records the measured duration of a fixed allowlisted operation without entity-specific
// metric labels.
//
// Parameters:
//   - name: Name of the setting, cookie, resource or provider being addressed.
//   - seconds: Measured operation duration in seconds.
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

// FFmpegActive applies a start or stop delta to the active-FFmpeg-process gauge.
//
// Parameters:
//   - delta: Positive or negative lifecycle count adjustment.
func FFmpegActive(delta float64) {
	if r := current.Load(); r != nil {
		r.custom.WithLabelValues("ffmpeg_active").Add(delta)
	}
}

// WSMessage counts realtime message kinds using the fixed protocol label set.
//
// Parameters:
//   - kind: Validated event, job, media or resource category.
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

// Profiling runs the profiling endpoint only on loopback when explicitly enabled by the
// operator.
//
// Parameters:
//   - ctx: Context controlling cancellation, deadlines and operation lifetime.
//
// Returns:
//   - result 1: nil on success; otherwise the validation, access, transport or persistence
//     failure.
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
	// Unregister on bind failure or normal return; do not retain a waiter for
	// the rest of the process lifetime when this server never started.
	stop := context.AfterFunc(ctx, func() { _ = server.Close() })
	defer stop()
	return server.ListenAndServe()
}

// boundedProfile serves a profiling request with a bounded duration and controlled diagnostic
// access.
//
// Parameters:
//   - next: Next HTTP handler or newer event, as defined by its type.
//
// Returns:
//   - result 1: The resulting handler.
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
