package biz

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

//go:generate mockgen -source=collect.go -destination=mock/mock_collect.go -package=mock

// CollectedItem is the normalized unit a Fetcher returns for one piece of
// external material, mapped onto the fields PostUsecase.Create consumes
// (REQ-05). It carries no transport detail: data/ fetchers translate feed
// entries, Telegram messages or extracted HTML into this shape, keeping biz/
// free of ent/http/sql (INV-T02).
type CollectedItem struct {
	ExternalID  string
	PublishedAt time.Time
	Author      *string
	Text        string
	Metadata    map[string]string
}

// Fetcher is the collection port: given a configured Source it returns the
// items published since the previous cycle (NS-04). Implementations live in
// data/ (RSS, web, Telegram); they map external responses to CollectedItem and
// return errors without logging — logging and retry belong to task/ (CON-02,
// INV-T04).
type Fetcher interface {
	Fetch(ctx context.Context, source Source) ([]CollectedItem, error)
}

// RSSFetcher and WebFetcher are per-type names for Fetcher. Their only purpose
// is to disambiguate Wire injection: every fetcher satisfies the bare Fetcher
// method set, so each collector binds to a distinct named port instead of an
// ambiguous *biz.Fetcher (DEC-06). Behaviour is identical to Fetcher.
type (
	RSSFetcher interface{ Fetcher }
	WebFetcher interface{ Fetcher }
)

// PostCreator is the persistence port CollectUsecase delegates to. It is
// satisfied by *PostUsecase, so collection reuses the single Create path that
// already provides idempotency by (source_id, external_id) (REQ-06) and the
// self-contained outbox event / cumulative no-event behaviour (REQ-09).
// CollectUsecase never writes posts or summary events directly (CTR-T03,
// INV-T01).
type PostCreator interface {
	Create(
		ctx context.Context,
		sourceID, externalID, text string,
		publishedAt time.Time,
		author *string,
		metadata map[string]string,
	) (Post, error)
}

// CollectUsecase normalizes collected material into posts. It is fetcher- and
// type-agnostic: a collector passes its source plus the fetcher for that type.
type CollectUsecase struct {
	creator PostCreator
}

// NewCollectUsecase creates a CollectUsecase.
func NewCollectUsecase(creator PostCreator) *CollectUsecase {
	return &CollectUsecase{creator: creator}
}

// CollectSource fetches material for one source, drops non-text items (REQ-10)
// and persists each text item through PostCreator.Create. A Fetch failure is
// wrapped and returned so the calling collector can log it and move on to the
// next source (REQ-07, FM-T01). A per-item ErrPostSourceNotFound — the source
// was removed between listing and saving — is skipped, not fatal (FM-T04); any
// other Create error aborts this source's cycle and is returned.
func (uc *CollectUsecase) CollectSource(ctx context.Context, source Source, fetcher Fetcher) error {
	items, err := fetcher.Fetch(ctx, source)
	if err != nil {
		return fmt.Errorf("fetch source %s: %w", source.ID, err)
	}

	for _, item := range FilterTextItems(items) {
		_, createErr := uc.creator.Create(
			ctx,
			source.ID,
			item.ExternalID,
			item.Text,
			item.PublishedAt,
			item.Author,
			item.Metadata,
		)
		if createErr != nil {
			if errors.Is(createErr, ErrPostSourceNotFound) {
				continue
			}
			return fmt.Errorf("create post for source %s item %s: %w", source.ID, item.ExternalID, createErr)
		}
	}

	return nil
}

// FilterTextItems returns only items carrying a non-empty text body, enforcing
// the text-only MVP rule (REQ-10, BR-05). Non-text material (image, video or
// file with no caption) is dropped before Create so it never produces a Post
// and never fails the cycle (INV-T05, FM-T06). Pure: the input is not mutated.
func FilterTextItems(items []CollectedItem) []CollectedItem {
	out := make([]CollectedItem, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.Text) != "" {
			out = append(out, item)
		}
	}
	return out
}
