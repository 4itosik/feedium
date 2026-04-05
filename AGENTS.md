See PROJECT.md for project description.
See ARCHITECTURE.md for architecture description.
 
## Stack
Go, Connect-go, GORM, PostgreSQL, slog.
UI: React, Tailwind CSS, go:embed (monorepo)
 
## Key commands
`go run ./cmd/feedium/main.go` - run project
`go run ./cmd/feedium` - run project (current entrypoint)
`go run ./cmd/feedium run` - run project (explicit command mode)
`go run ./cmd/feedium run migrate` - run DB migrations with goose from the same binary
`DATABASE_URL=postgres://... go run ./cmd/feedium run migrate` - run migrations with explicit DSN
`go generate ./...` - run code generation (proto, mocks) where go:generate is defined
`./scripts/gen-proto.sh` - generate protobuf/connect code (set `PROTOC_INCLUDE=/path/to/include` if auto-detect fails)
`go test ./...` - run tests
`go test -run TestHealthHandler ./internal/bootstrap` - run specific test: `-run TestName`
`go vet ./...` - Analyzes code for suspicious constructs
`golangci-lint run ./... -c .golangci.yml` - Run linter
 
## Conventions
- Solve the problem, not the consequence
- Consult with me when choosing a library
- Don't touch existing migrations
- Use generated mocks with `go.uber.org/mock` (`mockgen`), not handwritten mocks
- For packages under `internal/app/**/adapters/postgres`, write unit tests with DB mocking using `go-mocket` (integration tests are handled separately)
- For new code and new functionality, tests are mandatory with coverage > 80%
- If code generation inputs are changed, run `go generate ./...` and commit generated outputs
 
## Constraints
- Don't touch existing migrations
- Consult with me when choosing a library
