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

// TelegramMessage is one new Telegram message yielded by the userbot session
// (FT-009 DEC-08). It is transport-agnostic — no gotd types leak past data/.
// TgID is the channel/chat identifier used to resolve the configured Source
// (matches biz.TelegramChannelConfig/TelegramGroupConfig.TgID).
type TelegramMessage struct {
	TgID int64
	Item CollectedItem
}

// TelegramSession is the live userbot connection port (DEC-08, AG-02). The
// data/ implementation is gotd-backed; it blocks in Run delivering each new
// channel/group message to handle until ctx is cancelled. A handle error is
// logged by the collector and never stops the stream (FM-T01). data/ owns
// connection, auth and FLOOD_WAIT handling; biz/ and task/ stay gotd-free.
type TelegramSession interface {
	Run(ctx context.Context, handle func(context.Context, TelegramMessage) error) error
}

// ErrTelegramNotAuthorized is returned by the Telegram session when the stored
// session is missing or unauthorized. First-time login (phone code) is a manual
// operation (AG-02) and is never automated.
var ErrTelegramNotAuthorized = errors.New("telegram session not authorized")

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

// CollectSource fetches material for one source and persists it via
// CollectItems (REQ-05..10). A Fetch failure is wrapped and returned so the
// calling collector can log it and move on to the next source (REQ-07, FM-T01).
func (uc *CollectUsecase) CollectSource(ctx context.Context, source Source, fetcher Fetcher) error {
	items, err := fetcher.Fetch(ctx, source)
	if err != nil {
		return fmt.Errorf("fetch source %s: %w", source.ID, err)
	}
	return uc.CollectItems(ctx, source, items)
}

// CollectItems persists already-fetched material for a source: it drops
// non-text items (REQ-10) and saves each text item through PostCreator.Create,
// reusing the single Create path that provides idempotency by
// (source_id, external_id) (REQ-06) and the self-contained-outbox /
// cumulative-no-event behaviour (REQ-09). A per-item ErrPostSourceNotFound is
// skipped (FM-T04); any other Create error aborts the batch and is returned.
// The push Telegram collector calls this directly with one item per message
// (DEC-08); pull collectors reach it via CollectSource.
func (uc *CollectUsecase) CollectItems(ctx context.Context, source Source, items []CollectedItem) error {
	for _, item := range FilterTextItems(items) {
		_, createErr := uc.creator.Create(
			ctx, source.ID, item.ExternalID, item.Text, item.PublishedAt, item.Author, item.Metadata,
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
