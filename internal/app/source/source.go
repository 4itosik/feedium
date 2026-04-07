package source

import (
	"time"

	"github.com/google/uuid"
)

type Type string

const (
	TypeTelegramChannel Type = "telegram_channel"
	TypeTelegramGroup   Type = "telegram_group"
	TypeRSS             Type = "rss"
	TypeWebScraping     Type = "web_scraping"
)

type Source struct {
	ID        uuid.UUID
	Type      Type
	Name      string
	URL       string
	Config    map[string]any
	CreatedAt time.Time
	UpdatedAt time.Time
}

type ListFilter struct {
	Type     Type
	PageSize int
	Page     int
}
