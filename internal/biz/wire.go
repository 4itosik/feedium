package biz

import "github.com/google/wire"

// ProviderSet Wire provider set for biz package
//
//nolint:gochecknoglobals // Wire provider sets are intentionally global
var ProviderSet = wire.NewSet(
	NewSourceUsecase,
	NewPostUsecase,
	NewSummaryUsecase,
	NewInterestUsecase,
	NewScoreUsecase,
	NewFeedUsecase,
	NewCollectUsecase,
	NewReactionUsecase,
	// CollectUsecase persists collected posts through the existing Create path
	// (CTR-T03): bind the PostCreator port to *PostUsecase.
	wire.Bind(new(PostCreator), new(*PostUsecase)),
)
