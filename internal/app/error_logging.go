package app

import (
	postgresinfra "github.com/janickiy/meet-space/internal/infrastructure/postgres"
	"github.com/janickiy/meet-space/internal/operations"
	"github.com/prometheus/client_golang/prometheus"
	"gorm.io/gorm"
)

// Each process has a bounded, asynchronous writer. It must stop before its SQL
// pool closes; losing the error store must never affect application readiness.
func startErrorLogging(db *gorm.DB) (*operations.ErrorLogRecorder, func()) {
	recorder := operations.NewErrorRecorder(postgresinfra.NewErrorLogsRepository(db))
	restore := operations.SetErrorRecorder(recorder)
	return recorder, func() { restore(); recorder.Close() }
}

func registerErrorLoggingMetrics(runtime *operations.Runtime, recorder *operations.ErrorLogRecorder) {
	for _, counter := range []struct {
		name, help string
		value      func() float64
	}{
		{"recorder_error_events_accepted_total", "Sanitized errors accepted by the bounded log queue.", func() float64 { return float64(recorder.Stats().Accepted) }},
		{"recorder_error_events_written_total", "Sanitized errors persisted successfully.", func() float64 { return float64(recorder.Stats().Written) }},
		{"recorder_error_events_dropped_total", "Errors dropped when the bounded queue cannot accept or retain them.", func() float64 { return float64(recorder.Stats().Dropped) }},
	} {
		runtime.Registry.MustRegister(prometheus.NewCounterFunc(prometheus.CounterOpts{Name: counter.name, Help: counter.help}, counter.value))
	}
	runtime.Registry.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: "recorder_error_events_queued", Help: "Error events waiting for persistence."}, func() float64 { return float64(recorder.Stats().Queued) }))
}
