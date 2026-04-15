package data

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/4itosik/feedium/internal/biz"
	entgo "github.com/4itosik/feedium/internal/ent"
	"github.com/4itosik/feedium/internal/ent/post"
	"github.com/4itosik/feedium/internal/ent/source"
)

type postRepo struct {
	data *Data
}

// Compile-time assertion.
var _ biz.PostRepo = (*postRepo)(nil)

// NewPostRepo creates a new post repository.
//
//nolint:revive // unexported return is intentional for Wire injection
func NewPostRepo(data *Data) *postRepo {
	return &postRepo{data: data}
}

// Save creates a new post in the database using upsert (idempotent).
// If post with same (source_id, external_id) exists, returns existing without modification.
func (pr *postRepo) Save(ctx context.Context, post biz.Post) (biz.Post, error) {
	sourceID, err := uuid.Parse(post.SourceID)
	if err != nil {
		return biz.Post{}, fmt.Errorf("invalid source id: %w", err)
	}

	postID, err := uuid.Parse(post.ID)
	if err != nil {
		return biz.Post{}, fmt.Errorf("invalid post id: %w", err)
	}

	// Ensure metadata is not nil (use empty map as default)
	metadata := post.Metadata
	if metadata == nil {
		metadata = map[string]string{}
	}

	// Try to insert the post
	// If conflict occurs (same source_id + external_id), fetch the existing one
	entPost, err := pr.data.Ent.Post.Create().
		SetID(postID).
		SetSourceID(sourceID).
		SetExternalID(post.ExternalID).
		SetPublishedAt(post.PublishedAt).
		SetNillableAuthor(post.Author).
		SetText(post.Text).
		SetMetadata(metadata).
		SetCreatedAt(post.CreatedAt).
		SetUpdatedAt(post.UpdatedAt).
		Save(ctx)

	if err != nil {
		// Check for unique constraint violation (post already exists with same source_id + external_id)
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			// Unique violation - fetch existing post
			return pr.getBySourceAndExternalID(ctx, sourceID, post.ExternalID)
		}

		// Check for FK violation (source doesn't exist)
		if errors.As(err, &pqErr) && pqErr.Code == "23503" {
			return biz.Post{}, biz.ErrPostSourceNotFound
		}

		return biz.Post{}, fmt.Errorf("save post: %w", err)
	}

	// Get the source info for eager loading
	src, err := pr.data.Ent.Source.Get(ctx, entPost.SourceID)
	if err != nil {
		return biz.Post{}, fmt.Errorf("get source for post: %w", err)
	}

	return pr.mapEntToDomain(entPost, src), nil
}

// getBySourceAndExternalID fetches existing post by (source_id, external_id) with source info.
func (pr *postRepo) getBySourceAndExternalID(
	ctx context.Context,
	sourceID uuid.UUID,
	externalID string,
) (biz.Post, error) {
	entPost, err := pr.data.Ent.Post.Query().
		Where(
			post.SourceIDEQ(sourceID),
			post.ExternalIDEQ(externalID),
		).
		WithSource().
		Only(ctx)
	if err != nil {
		if entgo.IsNotFound(err) {
			return biz.Post{}, biz.ErrPostNotFound
		}
		return biz.Post{}, fmt.Errorf("fetch existing post: %w", err)
	}

	return pr.mapEntToDomainWithSource(entPost), nil
}

// Update updates an existing post in the database.
func (pr *postRepo) Update(ctx context.Context, post biz.Post) (biz.Post, error) {
	id, err := uuid.Parse(post.ID)
	if err != nil {
		return biz.Post{}, fmt.Errorf("invalid post id: %w", err)
	}

	updateOne := pr.data.Ent.Post.UpdateOneID(id).
		SetExternalID(post.ExternalID).
		SetPublishedAt(post.PublishedAt).
		SetText(post.Text).
		SetMetadata(post.Metadata).
		SetUpdatedAt(post.UpdatedAt)

	if post.Author != nil {
		updateOne.SetAuthor(*post.Author)
	} else {
		updateOne.ClearAuthor()
	}

	updated, err := updateOne.Save(ctx)
	if err != nil {
		if entgo.IsNotFound(err) {
			return biz.Post{}, biz.ErrPostNotFound
		}

		// Check for unique violation on (source_id, external_id)
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			return biz.Post{}, biz.ErrPostAlreadyExists
		}

		return biz.Post{}, fmt.Errorf("update post: %w", err)
	}

	// Get source for eager loading
	src, err := pr.data.Ent.Source.Get(ctx, updated.SourceID)
	if err != nil {
		return biz.Post{}, fmt.Errorf("get source for post: %w", err)
	}

	return pr.mapEntToDomain(updated, src), nil
}

// Delete deletes a post from the database.
func (pr *postRepo) Delete(ctx context.Context, id string) error {
	uid, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid post id: %w", err)
	}

	err = pr.data.Ent.Post.DeleteOneID(uid).Exec(ctx)
	if err != nil {
		if entgo.IsNotFound(err) {
			return biz.ErrPostNotFound
		}
		return fmt.Errorf("delete post: %w", err)
	}
	return nil
}

// Get retrieves a post by ID with eager-loaded source info.
func (pr *postRepo) Get(ctx context.Context, id string) (biz.Post, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return biz.Post{}, fmt.Errorf("invalid post id: %w", err)
	}

	entPost, err := pr.data.Ent.Post.Query().
		Where(post.IDEQ(uid)).
		WithSource().
		Only(ctx)
	if err != nil {
		if entgo.IsNotFound(err) {
			return biz.Post{}, biz.ErrPostNotFound
		}
		return biz.Post{}, fmt.Errorf("get post: %w", err)
	}

	return pr.mapEntToDomainWithSource(entPost), nil
}

// List retrieves posts with pagination and filtering.
func (pr *postRepo) List(ctx context.Context, filter biz.ListPostsFilter) (biz.ListPostsResult, error) {
	query := pr.data.Ent.Post.Query()

	// Apply source filter
	if filter.SourceID != "" {
		sourceID, err := uuid.Parse(filter.SourceID)
		if err != nil {
			return biz.ListPostsResult{}, fmt.Errorf("invalid source id: %w", err)
		}
		query = query.Where(post.SourceIDEQ(sourceID))
	}

	// Get sort field and direction
	sortField, descending := pr.getSortParams(filter)

	// Parse and apply pagination
	var decodedToken *pageTokenData
	if filter.PageToken != "" {
		var err error
		decodedToken, err = pr.parsePageToken(filter.PageToken)
		if err != nil {
			return biz.ListPostsResult{}, err
		}
	}

	query = pr.applyPagination(query, sortField, descending, decodedToken)

	// Fetch posts
	entPosts, err := pr.fetchPosts(ctx, query, filter.PageSize)
	if err != nil {
		return biz.ListPostsResult{}, err
	}

	// Batch load sources
	sourcesMap, err := pr.batchLoadSources(ctx, entPosts)
	if err != nil {
		return biz.ListPostsResult{}, err
	}

	// Build result
	return pr.buildResult(ctx, entPosts, sourcesMap, sortField, filter.PageSize), nil
}

// getSortParams returns sort field and direction.
func (pr *postRepo) getSortParams(filter biz.ListPostsFilter) (string, bool) {
	var sortField string
	switch filter.OrderBy {
	case biz.SortByPublishedAt:
		sortField = post.FieldPublishedAt
	case biz.SortByCreatedAt:
		sortField = post.FieldCreatedAt
	default:
		sortField = post.FieldPublishedAt
	}
	return sortField, filter.OrderDir == biz.SortDesc
}

// parsePageToken decodes page token if provided.
func (pr *postRepo) parsePageToken(token string) (*pageTokenData, error) {
	decoded, err := decodePageToken(token)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid page token", biz.ErrPostInvalidArgument)
	}
	return &decoded, nil
}

// applyPagination applies cursor and ordering to query.
func (pr *postRepo) applyPagination(
	query *entgo.PostQuery,
	sortField string,
	descending bool,
	token *pageTokenData,
) *entgo.PostQuery {
	if token != nil {
		if descending {
			query = pr.applyCursorFilterDesc(query, sortField, token)
		} else {
			query = pr.applyCursorFilterAsc(query, sortField, token)
		}
	}

	if descending {
		query = query.Order(entgo.Desc(sortField), entgo.Desc(post.FieldID))
	} else {
		query = query.Order(entgo.Asc(sortField), entgo.Asc(post.FieldID))
	}

	return query
}

// fetchPosts retrieves posts with limit (pageSize + 1).
func (pr *postRepo) fetchPosts(
	ctx context.Context,
	query *entgo.PostQuery,
	pageSize int,
) ([]*entgo.Post, error) {
	entPosts, err := query.Limit(pageSize + 1).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list posts: %w", err)
	}
	return entPosts, nil
}

// batchLoadSources loads sources for given posts in batch.
func (pr *postRepo) batchLoadSources(
	ctx context.Context,
	posts []*entgo.Post,
) (map[uuid.UUID]*entgo.Source, error) {
	if len(posts) == 0 {
		return make(map[uuid.UUID]*entgo.Source), nil
	}

	sourceIDs := make([]uuid.UUID, 0, len(posts))
	for _, p := range posts {
		sourceIDs = append(sourceIDs, p.SourceID)
	}

	sources, err := pr.data.Ent.Source.Query().
		Where(source.IDIn(sourceIDs...)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("batch load sources: %w", err)
	}

	sourcesMap := make(map[uuid.UUID]*entgo.Source, len(sources))
	for _, s := range sources {
		sourcesMap[s.ID] = s
	}
	return sourcesMap, nil
}

// buildResult constructs ListPostsResult from fetched data.
func (pr *postRepo) buildResult(
	ctx context.Context,
	entPosts []*entgo.Post,
	sourcesMap map[uuid.UUID]*entgo.Source,
	sortField string,
	pageSize int,
) biz.ListPostsResult {
	result := biz.ListPostsResult{
		Items:         []biz.Post{},
		NextPageToken: "",
	}

	returnCount := min(len(entPosts), pageSize)

	for i := range returnCount {
		src := sourcesMap[entPosts[i].SourceID]
		if src == nil {
			// Fallback: fetch individually (shouldn't happen)
			src, _ = pr.data.Ent.Source.Get(ctx, entPosts[i].SourceID)
		}
		result.Items = append(result.Items, pr.mapEntToDomain(entPosts[i], src))
	}

	// Set next page token if there are more results
	if len(entPosts) > pageSize {
		lastIndex := returnCount - 1
		sortValue := entPosts[lastIndex].CreatedAt
		if sortField == post.FieldPublishedAt {
			sortValue = entPosts[lastIndex].PublishedAt
		}

		nextToken, _ := encodePageToken(sortValue, entPosts[lastIndex].ID)
		result.NextPageToken = nextToken
	}

	return result
}

// applyCursorFilterDesc applies cursor filter for descending order.
func (pr *postRepo) applyCursorFilterDesc(
	query *entgo.PostQuery,
	sortField string,
	token *pageTokenData,
) *entgo.PostQuery {
	// For DESC: (sort_field < token.sort_value) OR (sort_field = token.sort_value AND id < token.id)
	switch sortField {
	case post.FieldPublishedAt:
		return query.Where(
			post.Or(
				post.PublishedAtLT(token.SortValue),
				post.And(
					post.PublishedAtEQ(token.SortValue),
					post.IDLT(token.ID),
				),
			),
		)
	case post.FieldCreatedAt:
		return query.Where(
			post.Or(
				post.CreatedAtLT(token.SortValue),
				post.And(
					post.CreatedAtEQ(token.SortValue),
					post.IDLT(token.ID),
				),
			),
		)
	default:
		return query
	}
}

// applyCursorFilterAsc applies cursor filter for ascending order.
func (pr *postRepo) applyCursorFilterAsc(
	query *entgo.PostQuery,
	sortField string,
	token *pageTokenData,
) *entgo.PostQuery {
	// For ASC: (sort_field > token.sort_value) OR (sort_field = token.sort_value AND id > token.id)
	switch sortField {
	case post.FieldPublishedAt:
		return query.Where(
			post.Or(
				post.PublishedAtGT(token.SortValue),
				post.And(
					post.PublishedAtEQ(token.SortValue),
					post.IDGT(token.ID),
				),
			),
		)
	case post.FieldCreatedAt:
		return query.Where(
			post.Or(
				post.CreatedAtGT(token.SortValue),
				post.And(
					post.CreatedAtEQ(token.SortValue),
					post.IDGT(token.ID),
				),
			),
		)
	default:
		return query
	}
}

// mapEntToDomain converts an ent.Post and ent.Source to a biz.Post.
func (pr *postRepo) mapEntToDomain(entPost *entgo.Post, entSource *entgo.Source) biz.Post {
	return biz.Post{
		ID:          entPost.ID.String(),
		SourceID:    entPost.SourceID.String(),
		Source:      biz.SourceInfo{ID: entSource.ID.String(), Type: biz.SourceType(entSource.Type)},
		ExternalID:  entPost.ExternalID,
		PublishedAt: entPost.PublishedAt,
		Author:      entPost.Author,
		Text:        entPost.Text,
		Metadata:    entPost.Metadata,
		CreatedAt:   entPost.CreatedAt,
		UpdatedAt:   entPost.UpdatedAt,
	}
}

// mapEntToDomainWithSource converts an ent.Post with eager-loaded source to a biz.Post.
func (pr *postRepo) mapEntToDomainWithSource(entPost *entgo.Post) biz.Post {
	src, _ := entPost.Edges.SourceOrErr()
	if src == nil {
		// Fallback - should not happen with WithSource()
		return biz.Post{
			ID:          entPost.ID.String(),
			SourceID:    entPost.SourceID.String(),
			ExternalID:  entPost.ExternalID,
			PublishedAt: entPost.PublishedAt,
			Author:      entPost.Author,
			Text:        entPost.Text,
			Metadata:    entPost.Metadata,
			CreatedAt:   entPost.CreatedAt,
			UpdatedAt:   entPost.UpdatedAt,
		}
	}

	return biz.Post{
		ID:          entPost.ID.String(),
		SourceID:    entPost.SourceID.String(),
		Source:      biz.SourceInfo{ID: src.ID.String(), Type: biz.SourceType(src.Type)},
		ExternalID:  entPost.ExternalID,
		PublishedAt: entPost.PublishedAt,
		Author:      entPost.Author,
		Text:        entPost.Text,
		Metadata:    entPost.Metadata,
		CreatedAt:   entPost.CreatedAt,
		UpdatedAt:   entPost.UpdatedAt,
	}
}
