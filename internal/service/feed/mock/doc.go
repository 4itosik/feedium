package mock

//go:generate go run go.uber.org/mock/mockgen -destination=mock_usecase.go -package=mock github.com/4itosik/feedium/internal/service/feed Usecase
