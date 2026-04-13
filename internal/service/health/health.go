package health

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"

	feediumv1 "github.com/4itosik/feedium/api/feedium"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type throttledLogger struct {
	logger *slog.Logger
	last   atomic.Int64
}

func (t *throttledLogger) LogError(ctx context.Context, err error) {
	now := time.Now().UnixNano()
	prev := t.last.Load()
	if now-prev < time.Second.Nanoseconds() {
		return
	}
	if t.last.CompareAndSwap(prev, now) {
		t.logger.ErrorContext(ctx, "healthz ping failed", "err", err)
	}
}

type HealthService struct {
	feediumv1.UnimplementedHealthServiceServer

	pinger   Pinger
	throttle *throttledLogger
}

func NewHealthService(pinger Pinger, logger *slog.Logger) *HealthService {
	return &HealthService{
		pinger:   pinger,
		throttle: &throttledLogger{logger: logger},
	}
}

func (h *HealthService) Check(ctx context.Context) (string, bool) {
	ctx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()

	err := h.pinger.Ping(ctx)
	if err == nil {
		return "ok", true
	}

	h.throttle.LogError(ctx, err)
	return "unavailable", false
}

func (h *HealthService) V1Check(ctx context.Context, _ *feediumv1.V1CheckRequest) (*feediumv1.V1CheckResponse, error) {
	statusStr, ok := h.Check(ctx)
	if ok {
		return &feediumv1.V1CheckResponse{Status: statusStr}, nil
	}
	return nil, status.Errorf(codes.Unavailable, "unavailable")
}
