package bootstrap

import (
	"io"
	"net/http"
)

func HealthHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if _, err := io.WriteString(w, `{"status":"ok"}`); err != nil {
		return
	}
}
