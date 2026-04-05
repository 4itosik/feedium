package bootstrap_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"feedium/internal/bootstrap"
)

func TestHealthHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	bootstrap.HealthHandler(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status %d", rr.Code)
	}
	if got := rr.Body.String(); got != `{"status":"ok"}` {
		t.Fatalf("body %q", got)
	}
}
