package biz

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"
)

// SourceType defines the type of source.
type SourceType string

const (
	SourceTypeTelegramChannel SourceType = "telegram_channel"
	SourceTypeTelegramGroup   SourceType = "telegram_group"
	SourceTypeRSS             SourceType = "rss"
	SourceTypeHTML            SourceType = "html"
)

// ProcessingMode defines how a source's content is processed.
type ProcessingMode string

const (
	ProcessingModeSelfContained ProcessingMode = "self_contained"
	ProcessingModeCumulative    ProcessingMode = "cumulative"

	maxPageSize = 500
	minPageSize = 1
)

// SourceConfig is a marker interface for type-specific configs.
type SourceConfig interface {
	sourceConfigMarker()
}

// TelegramChannelConfig holds configuration for Telegram channel source.
type TelegramChannelConfig struct {
	TgID     int64  `json:"tg_id"`
	Username string `json:"username"`
}

func (TelegramChannelConfig) sourceConfigMarker() {}

// TelegramGroupConfig holds configuration for Telegram group source.
type TelegramGroupConfig struct {
	TgID     int64  `json:"tg_id"`
	Username string `json:"username"`
}

func (TelegramGroupConfig) sourceConfigMarker() {}

// RSSConfig holds configuration for RSS source.
type RSSConfig struct {
	FeedURL string `json:"feed_url"`
}

func (RSSConfig) sourceConfigMarker() {}

// HTMLConfig holds configuration for HTML source.
type HTMLConfig struct {
	URL string `json:"url"`
}

func (HTMLConfig) sourceConfigMarker() {}

// Source represents a content source.
type Source struct {
	ID             string
	Type           SourceType
	ProcessingMode ProcessingMode
	Config         SourceConfig
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// ListSourcesFilter defines filtering and pagination for listing sources.
type ListSourcesFilter struct {
	Type           SourceType
	PageSize       int
	PageToken      string
	ProcessingMode *ProcessingMode
}

// ListSourcesResult holds results from listing sources.
type ListSourcesResult struct {
	Items         []Source
	NextPageToken string
}

// Sentinel errors for Source operations.
var (
	ErrSourceNotFound    = errors.New("source not found")
	ErrInvalidSourceType = errors.New("invalid source type")
	ErrInvalidConfig     = errors.New("invalid config")
	ErrTypeImmutable     = errors.New("source type cannot be changed")
)

// ValidateSourceType checks if the given type is valid.
func ValidateSourceType(t SourceType) error {
	switch t {
	case SourceTypeTelegramChannel, SourceTypeTelegramGroup, SourceTypeRSS, SourceTypeHTML:
		return nil
	}
	return ErrInvalidSourceType
}

// ValidateSourceConfig validates the configuration for the given source type.
func ValidateSourceConfig(t SourceType, cfg SourceConfig) error {
	if cfg == nil {
		return fmt.Errorf("%w: config is nil", ErrInvalidConfig)
	}

	switch t {
	case SourceTypeTelegramChannel:
		return validateTelegramChannelConfig(cfg)
	case SourceTypeTelegramGroup:
		return validateTelegramGroupConfig(cfg)
	case SourceTypeRSS:
		return validateRSSConfig(cfg)
	case SourceTypeHTML:
		return validateHTMLConfig(cfg)
	}

	return ErrInvalidSourceType
}

func validateTelegramChannelConfig(cfg SourceConfig) error {
	tcfg, ok := cfg.(*TelegramChannelConfig)
	if !ok {
		return fmt.Errorf("%w: expected TelegramChannelConfig", ErrInvalidConfig)
	}
	if tcfg.TgID == 0 {
		return fmt.Errorf("%w: tg_id cannot be 0", ErrInvalidConfig)
	}
	return nil
}

func validateTelegramGroupConfig(cfg SourceConfig) error {
	tgcfg, ok := cfg.(*TelegramGroupConfig)
	if !ok {
		return fmt.Errorf("%w: expected TelegramGroupConfig", ErrInvalidConfig)
	}
	if tgcfg.TgID == 0 {
		return fmt.Errorf("%w: tg_id cannot be 0", ErrInvalidConfig)
	}
	return nil
}

func validateRSSConfig(cfg SourceConfig) error {
	rcfg, ok := cfg.(*RSSConfig)
	if !ok {
		return fmt.Errorf("%w: expected RSSConfig", ErrInvalidConfig)
	}
	if rcfg.FeedURL == "" {
		return fmt.Errorf("%w: feed_url is required", ErrInvalidConfig)
	}
	if _, err := url.ParseRequestURI(rcfg.FeedURL); err != nil {
		return fmt.Errorf("%w: feed_url is invalid URL: %w", ErrInvalidConfig, err)
	}
	return nil
}

func validateHTMLConfig(cfg SourceConfig) error {
	hcfg, ok := cfg.(*HTMLConfig)
	if !ok {
		return fmt.Errorf("%w: expected HTMLConfig", ErrInvalidConfig)
	}
	if hcfg.URL == "" {
		return fmt.Errorf("%w: url is required", ErrInvalidConfig)
	}
	if _, err := url.ParseRequestURI(hcfg.URL); err != nil {
		return fmt.Errorf("%w: url is invalid URL: %w", ErrInvalidConfig, err)
	}
	return nil
}

// ProcessingModeForType returns the processing mode for a given source type.
// Based on BR-01: telegram_channel, rss, html -> self_contained; telegram_group -> cumulative.
func ProcessingModeForType(t SourceType) ProcessingMode {
	switch t {
	case SourceTypeTelegramChannel, SourceTypeRSS, SourceTypeHTML:
		return ProcessingModeSelfContained
	case SourceTypeTelegramGroup:
		return ProcessingModeCumulative
	}
	// Should not reach here if ValidateSourceType is called first
	return ProcessingModeSelfContained
}

// SourceRepo defines the repository interface for Source operations.
type SourceRepo interface {
	Save(ctx context.Context, source Source) (Source, error)
	Update(ctx context.Context, source Source) (Source, error)
	Delete(ctx context.Context, id string) error
	Get(ctx context.Context, id string) (Source, error)
	List(ctx context.Context, filter ListSourcesFilter) (ListSourcesResult, error)
}

// SourceUsecase handles business logic for Source operations.
type SourceUsecase struct {
	repo SourceRepo
}

// NewSourceUsecase creates a new SourceUsecase.
func NewSourceUsecase(repo SourceRepo) *SourceUsecase {
	return &SourceUsecase{repo: repo}
}

// Create creates a new source with validation.
func (uc *SourceUsecase) Create(ctx context.Context, sourceType SourceType, config SourceConfig) (Source, error) {
	// Validate source type
	if err := ValidateSourceType(sourceType); err != nil {
		return Source{}, err
	}

	// Validate config
	if err := ValidateSourceConfig(sourceType, config); err != nil {
		return Source{}, err
	}

	source := Source{
		ID:             "", // Will be set to UUID v7 by caller if needed
		Type:           sourceType,
		ProcessingMode: ProcessingModeForType(sourceType),
		Config:         config,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	return uc.repo.Save(ctx, source)
}

// Update updates an existing source with validation.
func (uc *SourceUsecase) Update(
	ctx context.Context,
	id string,
	sourceType SourceType,
	config SourceConfig,
) (Source, error) {
	// Get existing source
	existing, err := uc.repo.Get(ctx, id)
	if err != nil {
		return Source{}, err
	}

	// Check type immutability
	if existing.Type != sourceType {
		return Source{}, ErrTypeImmutable
	}

	// Validate config
	if cfgErr := ValidateSourceConfig(sourceType, config); cfgErr != nil {
		return Source{}, cfgErr
	}

	existing.Config = config
	existing.UpdatedAt = time.Now()

	return uc.repo.Update(ctx, existing)
}

// Delete deletes a source by ID.
func (uc *SourceUsecase) Delete(ctx context.Context, id string) error {
	return uc.repo.Delete(ctx, id)
}

// Get retrieves a source by ID.
func (uc *SourceUsecase) Get(ctx context.Context, id string) (Source, error) {
	return uc.repo.Get(ctx, id)
}

// List retrieves sources with pagination and filtering.
func (uc *SourceUsecase) List(ctx context.Context, filter ListSourcesFilter) (ListSourcesResult, error) {
	// Clamp page size
	if filter.PageSize < minPageSize {
		filter.PageSize = minPageSize
	}
	if filter.PageSize > maxPageSize {
		filter.PageSize = maxPageSize
	}

	// Validate type filter if provided
	if filter.Type != "" {
		if err := ValidateSourceType(filter.Type); err != nil {
			return ListSourcesResult{}, err
		}
	}

	return uc.repo.List(ctx, filter)
}
