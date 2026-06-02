package interest_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/4itosik/feedium/api/feedium"
	"github.com/4itosik/feedium/internal/biz"
	"github.com/4itosik/feedium/internal/service/interest"
	"github.com/4itosik/feedium/internal/service/interest/mock"
)

func TestInterestService_V1CreateInterest(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	t.Run("maps topic to usecase and domain back to proto", func(t *testing.T) {
		// Arrange
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		uc := mock.NewMockUsecase(ctrl)
		uc.EXPECT().Create(gomock.Any(), "ai").
			Return(biz.Interest{ID: "i-1", Topic: "ai", Model: "m"}, nil)
		svc := interest.NewInterestService(uc)

		// Act
		resp, err := svc.V1CreateInterest(context.Background(), &feedium.V1CreateInterestRequest{Topic: "ai"})

		// Assert
		require.NoError(t, err)
		assert.Equal(t, "i-1", resp.GetInterest().GetId())
		assert.Equal(t, "ai", resp.GetInterest().GetTopic())
	})

	t.Run("validation error maps to InvalidArgument", func(t *testing.T) {
		// Arrange
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		uc := mock.NewMockUsecase(ctrl)
		uc.EXPECT().Create(gomock.Any(), "").Return(biz.Interest{}, biz.ErrInterestValidation)
		svc := interest.NewInterestService(uc)

		// Act
		_, err := svc.V1CreateInterest(context.Background(), &feedium.V1CreateInterestRequest{Topic: ""})

		// Assert
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})
}

func TestInterestService_V1GetInterest(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	t.Run("not found maps to NotFound", func(t *testing.T) {
		// Arrange
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		uc := mock.NewMockUsecase(ctrl)
		uc.EXPECT().Get(gomock.Any(), "missing").Return(biz.Interest{}, biz.ErrInterestNotFound)
		svc := interest.NewInterestService(uc)

		// Act
		_, err := svc.V1GetInterest(context.Background(), &feedium.V1GetInterestRequest{Id: "missing"})

		// Assert
		assert.Equal(t, codes.NotFound, status.Code(err))
	})
}

func TestInterestService_V1UpdateInterest(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	t.Run("maps id and topic to usecase", func(t *testing.T) {
		// Arrange
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		uc := mock.NewMockUsecase(ctrl)
		uc.EXPECT().Update(gomock.Any(), "i-1", "devops").
			Return(biz.Interest{ID: "i-1", Topic: "devops"}, nil)
		svc := interest.NewInterestService(uc)

		// Act
		resp, err := svc.V1UpdateInterest(
			context.Background(),
			&feedium.V1UpdateInterestRequest{Id: "i-1", Topic: "devops"},
		)

		// Assert
		require.NoError(t, err)
		assert.Equal(t, "devops", resp.GetInterest().GetTopic())
	})
}

func TestInterestService_V1DeleteInterest(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	t.Run("delete delegates and returns empty response", func(t *testing.T) {
		// Arrange
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		uc := mock.NewMockUsecase(ctrl)
		uc.EXPECT().Delete(gomock.Any(), "i-1").Return(nil)
		svc := interest.NewInterestService(uc)

		// Act
		_, err := svc.V1DeleteInterest(context.Background(), &feedium.V1DeleteInterestRequest{Id: "i-1"})

		// Assert
		require.NoError(t, err)
	})

	t.Run("not found maps to NotFound", func(t *testing.T) {
		// Arrange
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		uc := mock.NewMockUsecase(ctrl)
		uc.EXPECT().Delete(gomock.Any(), "missing").Return(biz.ErrInterestNotFound)
		svc := interest.NewInterestService(uc)

		// Act
		_, err := svc.V1DeleteInterest(context.Background(), &feedium.V1DeleteInterestRequest{Id: "missing"})

		// Assert
		assert.Equal(t, codes.NotFound, status.Code(err))
	})
}

func TestInterestService_V1ListInterests(t *testing.T) {
	defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

	t.Run("maps items and next page token", func(t *testing.T) {
		// Arrange
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		uc := mock.NewMockUsecase(ctrl)
		uc.EXPECT().List(gomock.Any(), gomock.Any()).
			Return(biz.ListInterestsResult{
				Items:         []biz.Interest{{ID: "i-1", Topic: "ai"}},
				NextPageToken: "tok",
			}, nil)
		svc := interest.NewInterestService(uc)

		// Act
		resp, err := svc.V1ListInterests(context.Background(), &feedium.V1ListInterestsRequest{PageSize: 10})

		// Assert
		require.NoError(t, err)
		require.Len(t, resp.GetItems(), 1)
		assert.Equal(t, "tok", resp.GetNextPageToken())
	})
}
