package operations

import (
	"encoding/json"
	"net/http"
)

// ConfigureDrain registers the process-owned graceful-drain hooks used during deployment.
//
// Parameters:
//   - begin: Callback that stops acceptance of new component work.
//   - active: Callback returning the count of owned operations still active during drain.
func (r *Runtime) ConfigureDrain(begin func(), active func() int) {
	r.beginDrain, r.activeWork = begin, active
}

// DrainStatus returns the current drain state and active-work counts.
//
// Parameters:
//   - w: Destination HTTP response writer.
//   - req: Incoming HTTP request.
func (r *Runtime) DrainStatus(w http.ResponseWriter, req *http.Request) {
	if !Authorized(req, r.Config.MetricsSecret) {
		http.NotFound(w, req)
		return
	}
	if req.Method == http.MethodPost {
		r.Drain()
		if r.beginDrain != nil {
			r.beginDrain()
		}
	}
	active := int(r.inflight.Load())
	if r.activeWork != nil {
		active += r.activeWork()
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(struct {
		Draining bool `json:"draining"`
		Active   int  `json:"active"`
		Ready    bool `json:"ready"`
	}{r.draining.Load(), active, r.Ready()})
}
