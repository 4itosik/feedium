package mock

//go:generate go run go.uber.org/mock/mockgen -destination=mock_summary.go -package=mock github.com/4itosik/feedium/internal/service/summary Usecase
