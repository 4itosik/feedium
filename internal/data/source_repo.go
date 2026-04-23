package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/4itosik/feedium/internal/biz"
	entgo "github.com/4itosik/feedium/internal/ent"
	"github.com/4itosik/feedium/internal/ent/source"
)

type sourceRepo struct {
	data *Data
}

// Compile-time assertion.
var _ biz.SourceRepo = (*sourceRepo)(nil)

// NewSourceRepo creates a new source repository.
//
//nolint:revive // unexported return is intentional for Wire injection
func NewSourceRepo(data *Data) *sourceRepo {
	return &sourceRepo{data: data}
}

// Save creates a new source in the database.
func (sr *sourceRepo) Save(ctx context.Context, source biz.Source) (biz.Source, error) {
	// Serialize config to JSON
	configJSON, err := sr.serializeConfig(source.Config)
	if err != nil {
		return biz.Source{}, fmt.Errorf("serialize config: %w", err)
	}

	// Generate UUID if not set
	if source.ID == "" {
		source.ID = uuid.Must(uuid.NewV7()).String()
	}

	id, err := uuid.Parse(source.ID)
	if err != nil {
		return biz.Source{}, fmt.Errorf("invalid source id: %w", err)
	}

	entSource, err := sr.data.Ent.Source.Create().
		SetID(id).
		SetType(string(source.Type)).
		SetConfig(configJSON).
		SetCreatedAt(source.CreatedAt).
		SetUpdatedAt(source.UpdatedAt).
		Save(ctx)
	if err != nil {
		return biz.Source{}, fmt.Errorf("save source: %w", err)
	}

	return sr.mapEntToDomain(entSource)
}

// Update updates an existing source in the database.
func (sr *sourceRepo) Update(ctx context.Context, source biz.Source) (biz.Source, error) {
	// Serialize config to JSON
	configJSON, err := sr.serializeConfig(source.Config)
	if err != nil {
		return biz.Source{}, fmt.Errorf("serialize config: %w", err)
	}

	id, err := uuid.Parse(source.ID)
	if err != nil {
		return biz.Source{}, fmt.Errorf("invalid source id: %w", err)
	}

	entSource, err := sr.data.Ent.Source.UpdateOneID(id).
		SetConfig(configJSON).
		SetUpdatedAt(source.UpdatedAt).
		Save(ctx)
	if err != nil {
		if entgo.IsNotFound(err) {
			return biz.Source{}, biz.ErrSourceNotFound
		}
		return biz.Source{}, fmt.Errorf("update source: %w", err)
	}

	return sr.mapEntToDomain(entSource)
}

// Delete deletes a source from the database.
func (sr *sourceRepo) Delete(ctx context.Context, id string) error {
	uuid, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid source id: %w", err)
	}

	err = sr.data.Ent.Source.DeleteOneID(uuid).Exec(ctx)
	if err != nil {
		if entgo.IsNotFound(err) {
			return biz.ErrSourceNotFound
		}
		return fmt.Errorf("delete source: %w", err)
	}
	return nil
}

// Get retrieves a source by ID.
func (sr *sourceRepo) Get(ctx context.Context, id string) (biz.Source, error) {
	uuid, err := uuid.Parse(id)
	if err != nil {
		return biz.Source{}, fmt.Errorf("invalid source id: %w", err)
	}

	entSource, err := sr.data.Ent.Source.Get(ctx, uuid)
	if err != nil {
		if entgo.IsNotFound(err) {
			return biz.Source{}, biz.ErrSourceNotFound
		}
		return biz.Source{}, fmt.Errorf("get source: %w", err)
	}

	return sr.mapEntToDomain(entSource)
}

// List retrieves sources with pagination and filtering.
func (sr *sourceRepo) List(ctx context.Context, filter biz.ListSourcesFilter) (biz.ListSourcesResult, error) {
	query := sr.data.Ent.Source.Query()

	// Apply type filter if provided
	if filter.Type != "" {
		query = query.Where(source.TypeEQ(string(filter.Type)))
	}

	if filter.ProcessingMode != nil {
		switch *filter.ProcessingMode {
		case biz.ProcessingModeCumulative:
			query = query.Where(source.TypeEQ(string(biz.SourceTypeTelegramGroup)))
		case biz.ProcessingModeSelfContained:
			query = query.Where(source.TypeIn(
				string(biz.SourceTypeTelegramChannel),
				string(biz.SourceTypeRSS),
				string(biz.SourceTypeHTML),
			))
		}
	}

	// Parse page token if provided
	var decodedToken *pageTokenData

	if filter.PageToken != "" {
		decoded, err := decodePageToken(filter.PageToken)
		if err != nil {
			return biz.ListSourcesResult{}, fmt.Errorf("%w: invalid page token", biz.ErrInvalidConfig)
		}
		decodedToken = &decoded
	}

	// Order by created_at and id for stable pagination
	query = query.Order(entgo.Asc(source.FieldCreatedAt), entgo.Asc(source.FieldID))

	// Apply cursor-based pagination
	if decodedToken != nil {
		query = query.Where(
			source.Or(
				source.CreatedAtGT(decodedToken.SortValue),
				source.And(
					source.CreatedAtEQ(decodedToken.SortValue),
					source.IDGT(decodedToken.ID),
				),
			),
		)
	}

	// Fetch page_size + 1 to detect if there's a next page
	fetchLimit := filter.PageSize + 1
	entSources, err := query.Limit(fetchLimit).All(ctx)
	if err != nil {
		return biz.ListSourcesResult{}, fmt.Errorf("list sources: %w", err)
	}

	// Map to domain objects
	result := biz.ListSourcesResult{
		Items:         []biz.Source{},
		NextPageToken: "",
	}

	returnCount := min(len(entSources), filter.PageSize)

	for i := range returnCount {
		domainSource, mapErr := sr.mapEntToDomain(entSources[i])
		if mapErr != nil {
			return biz.ListSourcesResult{}, mapErr
		}
		result.Items = append(result.Items, domainSource)
	}

	// Set next page token if there are more results
	if len(entSources) > filter.PageSize {
		lastIndex := returnCount - 1
		nextToken, tokenErr := encodePageToken(entSources[lastIndex].CreatedAt, entSources[lastIndex].ID)
		if tokenErr != nil {
			return biz.ListSourcesResult{}, fmt.Errorf("encode page token: %w", tokenErr)
		}
		result.NextPageToken = nextToken
	}

	return result, nil
}

// ClaimDueCumulative atomically picks one cumulative source whose next_summary_at is due
// (NULL or <= now()) under FOR UPDATE SKIP LOCKED. Returns ErrNoSourceDue when nothing is due.
func (sr *sourceRepo) ClaimDueCumulative(ctx context.Context) (biz.Source, error) {
	ex := sqlExecerFromContext(ctx, sr.data.DB)

	// Cumulative mode is derived from source.type (telegram_group) — see ProcessingModeForType.
	// We can't SKIP LOCKED on a JOIN, but a plain CTE here is enough.
	query := `
WITH picked AS (
    SELECT id FROM sources
    WHERE type = $1
      AND (next_summary_at IS NULL OR next_summary_at <= now())
    ORDER BY COALESCE(next_summary_at, created_at) ASC
    LIMIT 1
    FOR UPDATE SKIP LOCKED
)
SELECT s.id, s.type, s.config, s.created_at, s.updated_at
FROM sources s
JOIN picked p ON p.id = s.id
`
	row := ex.QueryRowContext(ctx, query, string(biz.SourceTypeTelegramGroup))

	var (
		id        uuid.UUID
		typ       string
		configRaw []byte
		createdAt time.Time
		updatedAt time.Time
	)
	if err := row.Scan(&id, &typ, &configRaw, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return biz.Source{}, biz.ErrNoSourceDue
		}
		return biz.Source{}, fmt.Errorf("claim due cumulative: %w", err)
	}

	var cfgMap map[string]any
	if unmarshalErr := json.Unmarshal(configRaw, &cfgMap); unmarshalErr != nil {
		return biz.Source{}, fmt.Errorf("unmarshal config: %w", unmarshalErr)
	}

	cfg, cfgErr := sr.deserializeConfig(biz.SourceType(typ), cfgMap)
	if cfgErr != nil {
		return biz.Source{}, fmt.Errorf("deserialize config: %w", cfgErr)
	}

	return biz.Source{
		ID:             id.String(),
		Type:           biz.SourceType(typ),
		ProcessingMode: biz.ProcessingModeForType(biz.SourceType(typ)),
		Config:         cfg,
		CreatedAt:      createdAt,
		UpdatedAt:      updatedAt,
	}, nil
}

// BumpNextSummaryAt sets sources.next_summary_at for the given source id.
func (sr *sourceRepo) BumpNextSummaryAt(ctx context.Context, sourceID string, nextAt time.Time) error {
	ex := sqlExecerFromContext(ctx, sr.data.DB)

	sid, err := uuid.Parse(sourceID)
	if err != nil {
		return fmt.Errorf("invalid source id: %w", err)
	}

	res, err := ex.ExecContext(ctx, `UPDATE sources SET next_summary_at = $1 WHERE id = $2`, nextAt, sid)
	if err != nil {
		return fmt.Errorf("bump next_summary_at: %w", err)
	}
	affected, affErr := res.RowsAffected()
	if affErr != nil {
		return fmt.Errorf("bump rows affected: %w", affErr)
	}
	if affected == 0 {
		return biz.ErrSourceNotFound
	}
	return nil
}

// mapEntToDomain converts an ent.Source to a biz.Source.
func (sr *sourceRepo) mapEntToDomain(entSource *entgo.Source) (biz.Source, error) {
	sourceType := biz.SourceType(entSource.Type)

	// Validate type
	if err := biz.ValidateSourceType(sourceType); err != nil {
		return biz.Source{}, fmt.Errorf("invalid source type in database: %w", err)
	}

	// Deserialize config based on type
	config, err := sr.deserializeConfig(sourceType, entSource.Config)
	if err != nil {
		return biz.Source{}, fmt.Errorf("deserialize config: %w", err)
	}

	return biz.Source{
		ID:             entSource.ID.String(),
		Type:           sourceType,
		ProcessingMode: biz.ProcessingModeForType(sourceType),
		Config:         config,
		CreatedAt:      entSource.CreatedAt,
		UpdatedAt:      entSource.UpdatedAt,
	}, nil
}

// serializeConfig serializes a domain config to JSON.
func (sr *sourceRepo) serializeConfig(cfg biz.SourceConfig) (map[string]any, error) {
	data, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("marshal config: %w", err)
	}

	var result map[string]any
	if unmarshalErr := json.Unmarshal(data, &result); unmarshalErr != nil {
		return nil, fmt.Errorf("unmarshal config: %w", unmarshalErr)
	}

	return result, nil
}

// deserializeConfig deserializes a config from JSON based on the source type.
func (sr *sourceRepo) deserializeConfig(sourceType biz.SourceType, cfgMap map[string]any) (biz.SourceConfig, error) {
	// Re-marshal the config map to JSON for unmarshaling
	data, err := json.Marshal(cfgMap)
	if err != nil {
		return nil, fmt.Errorf("marshal config map: %w", err)
	}

	switch sourceType {
	case biz.SourceTypeTelegramChannel:
		var cfg biz.TelegramChannelConfig
		if unmarshalErr := json.Unmarshal(data, &cfg); unmarshalErr != nil {
			return nil, fmt.Errorf("unmarshal telegram channel config: %w", unmarshalErr)
		}
		return &cfg, nil

	case biz.SourceTypeTelegramGroup:
		var cfg biz.TelegramGroupConfig
		if unmarshalErr := json.Unmarshal(data, &cfg); unmarshalErr != nil {
			return nil, fmt.Errorf("unmarshal telegram group config: %w", unmarshalErr)
		}
		return &cfg, nil

	case biz.SourceTypeRSS:
		var cfg biz.RSSConfig
		if unmarshalErr := json.Unmarshal(data, &cfg); unmarshalErr != nil {
			return nil, fmt.Errorf("unmarshal rss config: %w", unmarshalErr)
		}
		return &cfg, nil

	case biz.SourceTypeHTML:
		var cfg biz.HTMLConfig
		if unmarshalErr := json.Unmarshal(data, &cfg); unmarshalErr != nil {
			return nil, fmt.Errorf("unmarshal html config: %w", unmarshalErr)
		}
		return &cfg, nil
	}

	return nil, fmt.Errorf("unknown source type: %s", sourceType)
}
