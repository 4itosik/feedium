package mock

//go:generate go run go.uber.org/mock/mockgen -destination=mock_health_client.go -package=mock github.com/4itosik/feedium/api/feedium HealthServiceClient
//go:generate go run go.uber.org/mock/mockgen -destination=mock_source_client.go -package=mock github.com/4itosik/feedium/api/feedium SourceServiceClient
