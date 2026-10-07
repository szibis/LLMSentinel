package localgateway

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestCIReservationAtomicWithRequestAdmission(t *testing.T) {
	g, err := New(Config{Upstream: "http://127.0.0.1:19091/v1", Timeout: 1000000000, MaxRequestBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	token := uuid.NewString()
	reserve := func(method, owner, remote, origin string) int {
		req := httptest.NewRequest(method, "/sentinel/ci-reservation", nil)
		req.RemoteAddr = remote
		req.Header.Set("X-Sentinel-CI-Token", owner)
		req.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		g.ServeHTTP(w, req)
		return w.Code
	}
	if reserve("POST", token, "192.0.2.1:1", "") != 403 {
		t.Fatal("remote control accepted")
	}
	if reserve("POST", token, "127.0.0.1:1", "https://example.com") != 403 {
		t.Fatal("browser control accepted")
	}
	g.admission.RLock() // An entire already accepted request, including its retries.
	if reserve("POST", token, "127.0.0.1:1", "") != 409 {
		t.Fatal("active request interrupted")
	}
	g.admission.RUnlock()
	if reserve("POST", token, "127.0.0.1:1", "") != 200 {
		t.Fatal("idle reservation failed")
	}
	for _, path := range []string{"/v1/messages", "/v1/responses", "/v1/chat/completions"} {
		w := httptest.NewRecorder()
		g.ServeHTTP(w, httptest.NewRequest("POST", path, strings.NewReader("{}")))
		if w.Code != 503 {
			t.Fatalf("new inference accepted during CI: %s %d", path, w.Code)
		}
	}
	if reserve("DELETE", uuid.NewString(), "127.0.0.1:1", "") != 409 {
		t.Fatal("wrong owner released reservation")
	}
	if reserve("DELETE", token, "127.0.0.1:1", "") != 200 {
		t.Fatal("owner could not release reservation")
	}
	w := httptest.NewRecorder()
	g.ServeHTTP(w, httptest.NewRequest("POST", "/v1/messages", strings.NewReader("{}")))
	if w.Code == 503 {
		t.Fatal("reservation not released")
	}
}
