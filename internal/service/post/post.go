// Package post provides the Post service implementation.
package post

import (
	"context"
	"errors"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/4itosik/feedium/api/feedium"
	"github.com/4itosik/feedium/internal/biz"
)

// Usecase defines the interface for Post business logic operations.
// This interface is defined here (where it's used) rather than in the biz package.
type Usecase interface {
	Create(
		ctx context.Context,
		sourceID, externalID, text string,
		publishedAt time.Time,
		author *string,
		metadata map[string]string,
	) (biz.Post, error)
	Update(
		ctx context.Context,
		id, externalID, text string,
		publishedAt time.Time,
		author *string,
		metadata map[string]string,
	) (biz.Post, error)
	Delete(ctx context.Context, id string) error
	Get(ctx context.Context, id string) (biz.Post, error)
	List(ctx context.Context, filter biz.ListPostsFilter) (biz.ListPostsResult, error)
}

// PostService implements the feedium.PostServiceServer.
//
//nolint:revive // exported type name Post+Service is not a stutter
type PostService struct {
	feedium.UnimplementedPostServiceServer

	uc Usecase
}

// NewPostService creates a new PostService.
func NewPostService(uc Usecase) *PostService {
	return &PostService{uc: uc}
}

// V1CreatePost creates a new post.
func (s *PostService) V1CreatePost(
	ctx context.Context,
	req *feedium.V1CreatePostRequest,
) (*feedium.V1CreatePostResponse, error) {
	// Default metadata to empty map if nil
	metadata := req.GetMetadata()
	if metadata == nil {
		metadata = map[string]string{}
	}

	// Map proto timestamp to time.Time
	publishedAt := req.GetPublishedAt().AsTime()

	// Call usecase
	post, err := s.uc.Create(
		ctx,
		req.GetSourceId(),
		req.GetExternalId(),
		req.GetText(),
		publishedAt,
		req.Author,
		metadata,
	)
	if err != nil {
		return nil, s.mapDomainErrorToStatus(err)
	}

	// Map domain to proto
	protoPost := s.mapDomainToProto(post)

	return &feedium.V1CreatePostResponse{Post: protoPost}, nil
}

// V1UpdatePost updates an existing post.
func (s *PostService) V1UpdatePost(
	ctx context.Context,
	req *feedium.V1UpdatePostRequest,
) (*feedium.V1UpdatePostResponse, error) {
	// Default metadata to empty map if nil
	metadata := req.GetMetadata()
	if metadata == nil {
		metadata = map[string]string{}
	}

	// Map proto timestamp to time.Time
	publishedAt := req.GetPublishedAt().AsTime()

	// Call usecase
	post, err := s.uc.Update(
		ctx,
		req.GetId(),
		req.GetExternalId(),
		req.GetText(),
		publishedAt,
		req.Author,
		metadata,
	)
	if err != nil {
		return nil, s.mapDomainErrorToStatus(err)
	}

	// Map domain to proto
	protoPost := s.mapDomainToProto(post)

	return &feedium.V1UpdatePostResponse{Post: protoPost}, nil
}

// V1DeletePost deletes a post.
func (s *PostService) V1DeletePost(
	ctx context.Context,
	req *feedium.V1DeletePostRequest,
) (*feedium.V1DeletePostResponse, error) {
	err := s.uc.Delete(ctx, req.GetId())
	if err != nil {
		return nil, s.mapDomainErrorToStatus(err)
	}

	return &feedium.V1DeletePostResponse{}, nil
}

// V1GetPost retrieves a post by ID.
func (s *PostService) V1GetPost(
	ctx context.Context,
	req *feedium.V1GetPostRequest,
) (*feedium.V1GetPostResponse, error) {
	post, err := s.uc.Get(ctx, req.GetId())
	if err != nil {
		return nil, s.mapDomainErrorToStatus(err)
	}

	// Map domain to proto
	protoPost := s.mapDomainToProto(post)

	return &feedium.V1GetPostResponse{Post: protoPost}, nil
}

// V1ListPosts lists posts with pagination and filtering.
func (s *PostService) V1ListPosts(
	ctx context.Context,
	req *feedium.V1ListPostsRequest,
) (*feedium.V1ListPostsResponse, error) {
	// Map proto enums to domain values
	orderBy := s.mapProtoSortFieldToDomain(req.GetOrderBy())
	orderDir := s.mapProtoSortDirectionToDomain(req.GetOrderDir())

	filter := biz.ListPostsFilter{
		SourceID:  req.GetSourceId(),
		PageSize:  int(req.GetPageSize()),
		PageToken: req.GetPageToken(),
		OrderBy:   orderBy,
		OrderDir:  orderDir,
	}

	result, err := s.uc.List(ctx, filter)
	if err != nil {
		return nil, s.mapDomainErrorToStatus(err)
	}

	// Map domain posts to proto
	protoItems := make([]*feedium.Post, 0, len(result.Items))
	for _, post := range result.Items {
		protoItems = append(protoItems, s.mapDomainToProto(post))
	}

	return &feedium.V1ListPostsResponse{
		Items:         protoItems,
		NextPageToken: result.NextPageToken,
	}, nil
}

// mapDomainToProto maps domain Post to proto Post.
func (s *PostService) mapDomainToProto(post biz.Post) *feedium.Post {
	return &feedium.Post{
		Id:          post.ID,
		Source:      s.mapDomainSourceToProto(post.Source),
		ExternalId:  post.ExternalID,
		PublishedAt: timestamppb.New(post.PublishedAt),
		Author:      post.Author,
		Text:        post.Text,
		Metadata:    post.Metadata,
		CreatedAt:   timestamppb.New(post.CreatedAt),
		UpdatedAt:   timestamppb.New(post.UpdatedAt),
	}
}

// mapDomainSourceToProto maps domain SourceInfo to proto PostSourceRef.
func (s *PostService) mapDomainSourceToProto(source biz.SourceInfo) *feedium.PostSourceRef {
	return &feedium.PostSourceRef{
		Id:   source.ID,
		Type: s.mapDomainTypeToProto(source.Type),
	}
}

// mapDomainTypeToProto maps domain SourceType to proto SourceType.
func (s *PostService) mapDomainTypeToProto(sourceType biz.SourceType) feedium.SourceType {
	switch sourceType {
	case biz.SourceTypeTelegramChannel:
		return feedium.SourceType_SOURCE_TYPE_TELEGRAM_CHANNEL
	case biz.SourceTypeTelegramGroup:
		return feedium.SourceType_SOURCE_TYPE_TELEGRAM_GROUP
	case biz.SourceTypeRSS:
		return feedium.SourceType_SOURCE_TYPE_RSS
	case biz.SourceTypeHTML:
		return feedium.SourceType_SOURCE_TYPE_HTML
	}
	return feedium.SourceType_SOURCE_TYPE_UNSPECIFIED
}

// mapProtoSortFieldToDomain maps proto PostSortField to domain SortField.
func (s *PostService) mapProtoSortFieldToDomain(protoSortField feedium.PostSortField) biz.SortField {
	switch protoSortField {
	case feedium.PostSortField_POST_SORT_FIELD_PUBLISHED_AT:
		return biz.SortByPublishedAt
	case feedium.PostSortField_POST_SORT_FIELD_CREATED_AT:
		return biz.SortByCreatedAt
	case feedium.PostSortField_POST_SORT_FIELD_UNSPECIFIED:
		return biz.SortByPublishedAt
	}
	// Default to SortByPublishedAt for any other values
	return biz.SortByPublishedAt
}

// mapProtoSortDirectionToDomain maps proto SortDirection to domain SortDirection.
func (s *PostService) mapProtoSortDirectionToDomain(protoSortDir feedium.SortDirection) biz.SortDirection {
	switch protoSortDir {
	case feedium.SortDirection_SORT_DIRECTION_ASC:
		return biz.SortAsc
	case feedium.SortDirection_SORT_DIRECTION_DESC:
		return biz.SortDesc
	case feedium.SortDirection_SORT_DIRECTION_UNSPECIFIED:
		return biz.SortDesc
	}
	// Default to SortDesc for any other values
	return biz.SortDesc
}

// mapDomainErrorToStatus maps domain errors to gRPC status errors.
func (s *PostService) mapDomainErrorToStatus(err error) error {
	switch {
	case errors.Is(err, biz.ErrPostNotFound):
		return status.Error(codes.NotFound, feedium.ErrorReason_ERROR_REASON_POST_NOT_FOUND.String())
	case errors.Is(err, biz.ErrPostInvalidArgument):
		return status.Error(codes.InvalidArgument, feedium.ErrorReason_ERROR_REASON_POST_INVALID_ARGUMENT.String())
	case errors.Is(err, biz.ErrPostSourceNotFound):
		return status.Error(codes.NotFound, feedium.ErrorReason_ERROR_REASON_POST_SOURCE_NOT_FOUND.String())
	case errors.Is(err, biz.ErrPostAlreadyExists):
		return status.Error(codes.AlreadyExists, feedium.ErrorReason_ERROR_REASON_POST_ALREADY_EXISTS.String())
	}
	return status.Error(codes.Internal, "internal error")
}
