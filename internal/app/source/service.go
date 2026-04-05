package source

import (
	"context"
	"log/slog"
	"net/url"
	"strings"

	"github.com/google/uuid"
)

type Service struct {
	repo Repository
	log  *slog.Logger
}

const (
	defaultPageSize = 50
	maxPageSize     = 100
)

func NewService(repo Repository, log *slog.Logger) *Service { return &Service{repo: repo, log: log} }

func (s *Service) Create(ctx context.Context, src *Source) (*Source, error) {
	if err := validateSource(src); err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, src); err != nil {
		return nil, err
	}
	return src, nil
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (*Source, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *Service) Update(ctx context.Context, src *Source) (*Source, error) {
	if err := validateSource(src); err != nil {
		return nil, err
	}
	if _, err := s.repo.GetByID(ctx, src.ID); err != nil {
		return nil, err
	}
	if err := s.repo.Update(ctx, src); err != nil {
		return nil, err
	}
	return src, nil
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error { return s.repo.Delete(ctx, id) }

func (s *Service) List(ctx context.Context, filter ListFilter) ([]Source, int, error) {
	filter.PageSize = normalizePageSize(filter.PageSize)
	filter.Page = normalizePage(filter.Page)
	return s.repo.List(ctx, filter)
}

func normalizePageSize(v int) int {
	if v <= 0 {
		return defaultPageSize
	}
	if v > maxPageSize {
		return maxPageSize
	}
	return v
}
func normalizePage(v int) int {
	if v <= 0 {
		return 1
	}
	return v
}

func validateSource(src *Source) error {
	if src == nil {
		return validationError("source is required")
	}
	src.Name = strings.TrimSpace(src.Name)
	src.URL = strings.TrimSpace(src.URL)
	if src.Name == "" || len(src.Name) > 255 {
		return validationError("invalid name")
	}
	if !isHTTPURL(src.URL) {
		return validationError("invalid url")
	}
	if src.Type == "" {
		return validationError("invalid type")
	}
	return validateConfig(src.Type, src.Config)
}

func isHTTPURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

func validateConfig(t Type, cfg map[string]any) error {
	if cfg == nil {
		return validationError("config is required")
	}
	switch t {
	case TypeTelegramChannel:
		v, ok := cfg["channel_id"].(string)
		if !ok || strings.TrimSpace(v) == "" {
			return validationError("invalid telegram channel config")
		}
	case TypeTelegramGroup:
		v, ok := cfg["group_id"].(string)
		if !ok || strings.TrimSpace(v) == "" {
			return validationError("invalid telegram group config")
		}
	case TypeRSS:
		v, ok := cfg["feed_url"].(string)
		if !ok || !isHTTPURL(strings.TrimSpace(v)) {
			return validationError("invalid rss config")
		}
	case TypeWebScraping:
		v, ok := cfg["selector"].(string)
		if !ok || strings.TrimSpace(v) == "" {
			return validationError("invalid web scraping config")
		}
	default:
		return validationError("invalid type")
	}
	return nil
}

type ValidationError struct{ msg string }

func (e ValidationError) Error() string { return e.msg }

func validationError(msg string) error { return ValidationError{msg: msg} }
