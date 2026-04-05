package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"feedium/internal/app/source"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Repository struct{ db *gorm.DB }

func New(db *gorm.DB) *Repository { return &Repository{db: db} }

func (r *Repository) Create(ctx context.Context, src *source.Source) error {
	row := fromDomain(src)
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return err
	}
	applyRow(src, &row)
	return nil
}

func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*source.Source, error) {
	var row sourceRow
	if err := r.db.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, source.ErrNotFound
		}
		return nil, err
	}
	out := toDomain(&row)
	return &out, nil
}

func (r *Repository) Update(ctx context.Context, src *source.Source) error {
	row := fromDomain(src)
	res := r.db.WithContext(ctx).Model(&sourceRow{}).Where("id = ?", src.ID).Updates(map[string]any{
		"type": row.Type, "name": row.Name, "url": row.URL, "config": row.Config, "updated_at": time.Now().UTC(),
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return source.ErrNotFound
	}
	return nil
}

func (r *Repository) Delete(ctx context.Context, id uuid.UUID) error {
	res := r.db.WithContext(ctx).Delete(&sourceRow{}, "id = ?", id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return source.ErrNotFound
	}
	return nil
}

func (r *Repository) List(ctx context.Context, filter source.ListFilter) ([]source.Source, int, error) {
	var rows []sourceRow
	q := r.db.WithContext(ctx).Model(&sourceRow{})
	if filter.Type != "" {
		q = q.Where("type = ?", filter.Type)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset := (filter.Page - 1) * filter.PageSize
	if err := q.Order("created_at desc").Limit(filter.PageSize).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	out := make([]source.Source, 0, len(rows))
	for i := range rows {
		s := toDomain(&rows[i])
		out = append(out, s)
	}
	return out, int(total), nil
}

type sourceRow struct {
	ID        uuid.UUID       `gorm:"type:uuid;default:gen_random_uuid();primaryKey"`
	Type      string          `gorm:"type:varchar(32);not null"`
	Name      string          `gorm:"type:varchar(255);not null"`
	URL       string          `gorm:"type:text;not null"`
	Config    json.RawMessage `gorm:"type:jsonb;not null;default:'{}'"`
	CreatedAt time.Time       `gorm:"type:timestamptz;not null;autoCreateTime"`
	UpdatedAt time.Time       `gorm:"type:timestamptz;not null;autoUpdateTime"`
}

func (sourceRow) TableName() string {
	return "sources"
}

func fromDomain(src *source.Source) sourceRow {
	cfg, _ := json.Marshal(src.Config)
	return sourceRow{
		ID:        src.ID,
		Type:      string(src.Type),
		Name:      src.Name,
		URL:       src.URL,
		Config:    cfg,
		CreatedAt: src.CreatedAt,
		UpdatedAt: src.UpdatedAt,
	}
}

func toDomain(row *sourceRow) source.Source {
	var cfg map[string]any
	_ = json.Unmarshal(row.Config, &cfg)
	return source.Source{
		ID:        row.ID,
		Type:      source.Type(row.Type),
		Name:      row.Name,
		URL:       row.URL,
		Config:    cfg,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
	}
}

func applyRow(dst *source.Source, row *sourceRow) {
	dst.ID, dst.CreatedAt, dst.UpdatedAt = row.ID, row.CreatedAt, row.UpdatedAt
}
