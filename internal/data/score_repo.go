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

// Upsert writes the single current score for a post in place (INV-02, CTR-02).
func (r *scoreRepo) Upsert(ctx context.Context, score biz.Score) (biz.Score, error) {
	ex := sqlExecerFromContext(ctx, r.data.DB)

	id, err := uuid.Parse(score.ID)
	if err != nil {
		return biz.Score{}, fmt.Errorf("invalid score id: %w", err)
	}
	postID, err := uuid.Parse(score.PostID)
	if err != nil {
		return biz.Score{}, fmt.Errorf("invalid post id: %w", err)
	}

	const query = `
INSERT INTO scores (id, post_id, value, model, created_at, updated_at)
VALUES ($1, $2, $3, $4, now(), now())
ON CONFLICT (post_id) DO UPDATE
SET value = EXCLUDED.value, model = EXCLUDED.model, updated_at = now()
RETURNING id, post_id, value, model, created_at, updated_at
`
	row := ex.QueryRowContext(ctx, query, id, postID, score.Value, score.Model)

	var (
		outID, outPostID uuid.UUID
		value            float64
		model            string
		createdAt        time.Time
		updatedAt        time.Time
	)
	if scanErr := row.Scan(&outID, &outPostID, &value, &model, &createdAt, &updatedAt); scanErr != nil {
		return biz.Score{}, fmt.Errorf("upsert score: %w", scanErr)
	}
	return biz.Score{
		ID:        outID.String(),
		PostID:    outPostID.String(),
		Value:     value,
		Model:     model,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}, nil
}

// ListRanked returns posts joined with their scores, ordered by score descending
// then post id, filtered by published_at range and min score, with keyset
// pagination (F3, DEC-06).
func (r *scoreRepo) ListRanked(ctx context.Context, query biz.FeedQuery) (biz.FeedResult, error) {
	ex := sqlExecerFromContext(ctx, r.data.DB)

	var (
		conds = []string{"s.value >= $1"}
		args  = []any{query.MinScore}
	)
	if query.From != nil {
		args = append(args, *query.From)
		conds = append(conds, fmt.Sprintf("p.published_at >= $%d", len(args)))
	}
	if query.To != nil {
		args = append(args, *query.To)
		conds = append(conds, fmt.Sprintf("p.published_at <= $%d", len(args)))
	}
	if query.PageToken != "" {
		cursor, err := decodeScoreToken(query.PageToken)
		if err != nil {
			return biz.FeedResult{}, fmt.Errorf("%w: invalid page token", biz.ErrInvalidThreshold)
		}
		args = append(args, cursor.value, cursor.id)
		// Keyset over (value DESC, p.id ASC): strictly after the cursor row.
		conds = append(conds, fmt.Sprintf("(s.value < $%d OR (s.value = $%d AND p.id > $%d))",
			len(args)-1, len(args)-1, len(args)))
	}
	args = append(args, query.PageSize+1)

	sqlText := `
SELECT p.id, p.source_id, p.author, COALESCE(sm.text, p.text), p.published_at, s.value,
       COALESCE(p.metadata->>'link', ''),
       COALESCE(src.config->'telegramChannel'->>'username',
                src.config->'telegramGroup'->>'username', ''),
       COALESCE(p.external_id, ''),
       COALESCE(pr.signal, '')
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
WHERE ` + strings.Join(conds, " AND ") + ` AND COALESCE(pr.signal, '') != 'dislike'
ORDER BY (CASE WHEN pr.signal = 'like' THEN s.value + 0.1 ELSE s.value END) DESC, p.id ASC
LIMIT $` + strconv.Itoa(len(args))

	rows, err := ex.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return biz.FeedResult{}, fmt.Errorf("list ranked feed: %w", err)
	}
	defer rows.Close()

	var items []biz.FeedItem
	for rows.Next() {
		var (
			postID, sourceID uuid.UUID
			author           sql.NullString
			text             string
			publishedAt      time.Time
			value            float64
			metaLink         string
			tgUsername       string
			externalID       string
			reactionSignal   string
		)
		if scanErr := rows.Scan(&postID, &sourceID, &author, &text, &publishedAt, &value,
			&metaLink, &tgUsername, &externalID, &reactionSignal); scanErr != nil {
			return biz.FeedResult{}, fmt.Errorf("scan feed row: %w", scanErr)
		}

		url := metaLink
		if url == "" && tgUsername != "" && externalID != "" {
			url = "https://t.me/" + tgUsername + "/" + externalID
		}

		post := biz.Post{
			ID:          postID.String(),
			SourceID:    sourceID.String(),
			Text:        text,
			PublishedAt: publishedAt,
		}
		if author.Valid {
			a := author.String
			post.Author = &a
		}
		items = append(items, biz.FeedItem{Post: post, Score: value, URL: url, Reaction: reactionSignal})
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
