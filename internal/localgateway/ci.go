package localgateway

import (
	"encoding/json"
	"net"
	"net/http"

	"github.com/google/uuid"
)

// Admission covers complete HTTP exchanges, including queued requests and retries.
// A reservation fails immediately if any inference request has been accepted.
func (g *Gateway) reserveForCI(w http.ResponseWriter, r *http.Request) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	ip := net.ParseIP(host)
	if err != nil || ip == nil || !ip.IsLoopback() || r.Header.Get("Origin") != "" {
		apiError(w, 403, "local_ci_only", "CI reservation requires a local non-browser request")
		return
	}
	if r.Method != http.MethodPost && r.Method != http.MethodDelete {
		apiError(w, 405, "method_not_allowed", "Use POST or DELETE")
		return
	}
	token := r.Header.Get("X-Sentinel-CI-Token")
	if _, err := uuid.Parse(token); err != nil {
		apiError(w, 400, "invalid_ci_token", "CI ownership token required")
		return
	}
	if !g.admission.TryLock() {
		apiError(w, 409, "lab_busy", "Accepted requests prevent CI reservation")
		return
	}
	defer g.admission.Unlock()
	if g.ciReservation != "" && g.ciReservation != token {
		apiError(w, 409, "ci_reserved", "Another CI reservation owns admission")
		return
	}
	if r.Method == http.MethodDelete {
		g.ciReservation = ""
	} else {
		g.ciReservation = token
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"reserved": g.ciReservation != ""})
}
