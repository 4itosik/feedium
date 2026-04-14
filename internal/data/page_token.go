package data

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const tokenParts = 2

// pageTokenData holds the decoded page token data.
type pageTokenData struct {
	CreatedAt time.Time
	ID        uuid.UUID
}

// encodePageToken encodes a page token from created_at and id.
//
//nolint:unparam // error return kept for symmetry with decodePageToken
func encodePageToken(createdAt time.Time, id uuid.UUID) (string, error) {
	// Format: RFC3339Nano + "|" + UUID
	tokenStr := fmt.Sprintf("%s|%s", createdAt.Format(time.RFC3339Nano), id.String())
	encoded := base64.StdEncoding.EncodeToString([]byte(tokenStr))
	return encoded, nil
}

// decodePageToken decodes a page token into created_at and id.
func decodePageToken(token string) (pageTokenData, error) {
	decoded, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return pageTokenData{}, fmt.Errorf("decode token: %w", err)
	}

	parts := strings.Split(string(decoded), "|")
	if len(parts) != tokenParts {
		return pageTokenData{}, errors.New("invalid token format")
	}

	createdAt, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return pageTokenData{}, fmt.Errorf("parse timestamp: %w", err)
	}

	id, err := uuid.Parse(parts[1])
	if err != nil {
		return pageTokenData{}, fmt.Errorf("parse uuid: %w", err)
	}

	return pageTokenData{
		CreatedAt: createdAt,
		ID:        id,
	}, nil
}
