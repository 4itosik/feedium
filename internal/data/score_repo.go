package data

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/4itosik/feedium/internal/biz"
)

type scoreRepo struct {
	data *Data
}

var (
	_ biz.ScoreRepo     = (*scoreRepo)(nil)
	_ biz.FeedQueryRepo = (*scoreRepo)(nil)
)

// NewScoreRepo creates a new score repository serving both the upsert path
// (ScoreRepo) and the ranked-feed read path (FeedQueryRepo).
//
//nolint:revive // unexported return type for wire DI
func NewScoreRepo(data *Data) *scoreRepo {
	return &scoreRepo{data: data}
}

// scoreUpsertPost writes the single current score for a post, matching the partial
// unique index idx_scores_post_id (WHERE post_id IS NOT NULL).
const scoreUpsertPost = `
INSERT INTO scores (id, post_id, value, model, created_at, updated_at)
VALUES ($1, $2, $3, $4, now(), now())
ON CONFLICT (post_id) WHERE post_id IS NOT NULL DO UPDATE
SET value = EXCLUDED.value, model = EXCLUDED.model, updated_at = now()
RETURNING id, post_id, summary_id, value, model, created_at, updated_at
`

// scoreUpsertDigest writes the single current score for a cumulative digest, matching
// the partial unique index idx_scores_summary_id (WHERE summary_id IS NOT NULL).
const scoreUpsertDigest = `
INSERT INTO scores (id, summary_id, value, model, created_at, updated_at)
VALUES ($1, $2, $3, $4, now(), now())
ON CONFLICT (summary_id) WHERE summary_id IS NOT NULL DO UPDATE
SET value = EXCLUDED.value, model = EXCLUDED.model, updated_at = now()
RETURNING id, post_id, summary_id, value, model, created_at, updated_at
`

// Upsert writes the single current score for a post or a digest in place (INV-02,
// CTR-01). Exactly one of PostID / SummaryID is set on the Score.
func (r *scoreRepo) Upsert(ctx context.Context, score biz.Score) (biz.Score, error) {
	ex := sqlExecerFromContext(ctx, r.data.DB)

	id, err := uuid.Parse(score.ID)
	if err != nil {
		return biz.Score{}, fmt.Errorf("invalid score id: %w", err)
	}

	var (
		query    string
		entityID uuid.UUID
	)
	switch {
	case score.PostID != nil:
		if entityID, err = uuid.Parse(*score.PostID); err != nil {
			return biz.Score{}, fmt.Errorf("invalid post id: %w", err)
		}
		query = scoreUpsertPost
	case score.SummaryID != nil:
		if entityID, err = uuid.Parse(*score.SummaryID); err != nil {
			return biz.Score{}, fmt.Errorf("invalid summary id: %w", err)
		}
		query = scoreUpsertDigest
	default:
		return biz.Score{}, errors.New("score has neither post_id nor summary_id")
	}

	row := ex.QueryRowContext(ctx, query, id, entityID, score.Value, score.Model)

	var (
		outID     uuid.UUID
		outPostID uuid.NullUUID
		outSumID  uuid.NullUUID
		value     float64
		model     string
		createdAt time.Time
		updatedAt time.Time
	)
	if scanErr := row.Scan(&outID, &outPostID, &outSumID, &value, &model, &createdAt, &updatedAt); scanErr != nil {
		return biz.Score{}, fmt.Errorf("upsert score: %w", scanErr)
	}
	out := biz.Score{
		ID:        outID.String(),
		Value:     value,
		Model:     model,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}
	if outPostID.Valid {
		s := outPostID.UUID.String()
		out.PostID = &s
	}
	if outSumID.Valid {
		s := outSumID.UUID.String()
		out.SummaryID = &s
	}
	return out, nil
}

// rankedFeedQueryHead is the static head of the ranked-feed query up to the dynamic
// WHERE clause. The feed is a UNION ALL of the post branch (unchanged: INNER JOIN
// posts, reaction like-boost / dislike-hide, latest-summary text) and the digest
// branch (scores keyed by summary_id over cumulative summaries, post_id IS NULL).
// Filters, keyset pagination and ordering are applied once over the union so posts
// and digests interleave by score with a single cursor (FT-020 REQ-04, CTR-03).
// `value` is the base score (keyset/cursor column, preserving post behavior);
// `effective_value` carries the post like-boost and is the ordering column (digests
// never have reactions, so their effective_value == value).
const rankedFeedQueryHead = `
SELECT q.id, q.source_id, q.author, q.text, q.published_at, q.value,
       q.meta_link, q.tg_username, q.external_id, q.reaction, q.kind
FROM (
    SELECT p.id AS id, p.source_id AS source_id, p.author AS author,
           COALESCE(sm.text, p.text) AS text, p.published_at AS published_at, s.value AS value,
           COALESCE(p.metadata->>'link', '') AS meta_link,
           COALESCE(src.config->'telegramChannel'->>'username',
                    src.config->'telegramGroup'->>'username', '') AS tg_username,
           COALESCE(p.external_id, '') AS external_id,
           COALESCE(pr.signal, '') AS reaction,
           (CASE WHEN pr.signal = 'like' THEN s.value + 0.1 ELSE s.value END) AS effective_value,
           'post' AS kind
    FROM scores s
    JOIN posts p ON p.id = s.post_id
    LEFT JOIN sources src ON src.id = p.source_id
    LEFT JOIN post_reactions pr ON pr.post_id = p.id
    LEFT JOIN LATERAL (
        SELECT text FROM summaries
        WHERE post_id = p.id
        ORDER BY created_at DESC
        LIMIT 1
    ) sm ON true
    WHERE s.post_id IS NOT NULL AND COALESCE(pr.signal, '') != 'dislike'
    UNION ALL
    SELECT su.id AS id, su.source_id AS source_id, NULL::text AS author,
           su.text AS text, su.created_at AS published_at, s.value AS value,
           '' AS meta_link,
           '' AS tg_username,
           '' AS external_id,
           '' AS reaction,
           s.value AS effective_value,
           'digest' AS kind
    FROM scores s
    JOIN summaries su ON su.id = s.summary_id
    WHERE s.summary_id IS NOT NULL AND su.post_id IS NULL
) q
WHERE `

// ListRanked returns posts and cumulative digests joined with their scores, ordered
// by score descending then id, filtered by published_at range and min score, with
// keyset pagination (F3, DEC-06, FT-020 REQ-04).
func (r *scoreRepo) ListRanked(ctx context.Context, query biz.FeedQuery) (biz.FeedResult, error) {
	ex := sqlExecerFromContext(ctx, r.data.DB)

	var (
		conds = []string{"q.value >= $1"}
		args  = []any{query.MinScore}
	)
	if query.From != nil {
		args = append(args, *query.From)
		conds = append(conds, fmt.Sprintf("q.published_at >= $%d", len(args)))
	}
	if query.To != nil {
		args = append(args, *query.To)
		conds = append(conds, fmt.Sprintf("q.published_at <= $%d", len(args)))
	}
	if query.PageToken != "" {
		cursor, err := decodeScoreToken(query.PageToken)
		if err != nil {
			return biz.FeedResult{}, fmt.Errorf("%w: invalid page token", biz.ErrInvalidThreshold)
		}
		args = append(args, cursor.value, cursor.id)
		// Keyset over (value DESC, id ASC): strictly after the cursor row.
		conds = append(conds, fmt.Sprintf("(q.value < $%d OR (q.value = $%d AND q.id > $%d))",
			len(args)-1, len(args)-1, len(args)))
	}
	args = append(args, query.PageSize+1)

	sqlText := rankedFeedQueryHead + strings.Join(conds, " AND ") +
		"\nORDER BY q.effective_value DESC, q.id ASC\nLIMIT $" + strconv.Itoa(len(args))

	rows, err := ex.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return biz.FeedResult{}, fmt.Errorf("list ranked feed: %w", err)
	}
	defer rows.Close()

	var items []biz.FeedItem
	for rows.Next() {
		var (
			entityID, sourceID uuid.UUID
			author             sql.NullString
			text               string
			publishedAt        time.Time
			value              float64
			metaLink           string
			tgUsername         string
			externalID         string
			reactionSignal     string
			kind               string
		)
		if scanErr := rows.Scan(&entityID, &sourceID, &author, &text, &publishedAt, &value,
			&metaLink, &tgUsername, &externalID, &reactionSignal, &kind); scanErr != nil {
			return biz.FeedResult{}, fmt.Errorf("scan feed row: %w", scanErr)
		}

		url := metaLink
		if url == "" && tgUsername != "" && externalID != "" {
			url = "https://t.me/" + tgUsername + "/" + externalID
		}

		// For a digest item, id is the summary id (stable identity for the keyset
		// cursor and the UI key), not a real posts row (CTR-03).
		post := biz.Post{
			ID:          entityID.String(),
			SourceID:    sourceID.String(),
			Text:        text,
			PublishedAt: publishedAt,
		}
		if author.Valid {
			a := author.String
			post.Author = &a
		}
		items = append(items, biz.FeedItem{
			Post:     post,
			Score:    value,
			URL:      url,
			Reaction: reactionSignal,
			Kind:     biz.FeedItemKind(kind),
		})
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return biz.FeedResult{}, fmt.Errorf("rows feed: %w", rowsErr)
	}

	result := biz.FeedResult{Items: []biz.FeedItem{}}
	returnCount := min(len(items), query.PageSize)
	result.Items = append(result.Items, items[:returnCount]...)
	if len(items) > query.PageSize {
		last := items[returnCount-1]
		result.NextPageToken = encodeScoreToken(last.Score, last.Post.ID)
	}
	return result, nil
}

// scoreToken is the keyset cursor for the ranked feed: (value, post id).
type scoreToken struct {
	value float64
	id    uuid.UUID
}

func encodeScoreToken(value float64, postID string) string {
	raw := fmt.Sprintf("%v|%s", value, postID)
	return base64.StdEncoding.EncodeToString([]byte(raw))
}

func decodeScoreToken(token string) (scoreToken, error) {
	decoded, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return scoreToken{}, fmt.Errorf("decode token: %w", err)
	}
	parts := strings.Split(string(decoded), "|")
	if len(parts) != tokenParts {
		return scoreToken{}, errors.New("invalid token format")
	}
	value, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return scoreToken{}, fmt.Errorf("parse value: %w", err)
	}
	id, err := uuid.Parse(parts[1])
	if err != nil {
		return scoreToken{}, fmt.Errorf("parse uuid: %w", err)
	}
	return scoreToken{value: value, id: id}, nil
}
