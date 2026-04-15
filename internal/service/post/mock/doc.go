package mock

//go:generate go run go.uber.org/mock/mockgen -destination=mock_post.go -package=mock github.com/4itosik/feedium/internal/service/post Usecase
