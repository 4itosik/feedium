See PROJECT.md for project description.
See ARCHITECTURE.md for architecture description.
 
## Stack
Go, Connect-go, GORM, PostgreSQL, slog.
UI: React, Tailwind CSS, go:embed (monorepo)
 
## Key commands
`go run ./cmd/feedium` - run project (current entrypoint)
`go run ./cmd/feedium run` - run project (explicit command mode)
`go run ./cmd/feedium run migrate` - run DB migrations with goose from the same binary
`DATABASE_URL=postgres://... go run ./cmd/feedium run migrate` - run migrations with explicit DSN
`go generate ./...` - run code generation (proto, mocks) where go:generate is defined
`./scripts/gen-proto.sh` - generate protobuf/connect code (set `PROTOC_INCLUDE=/path/to/include` if auto-detect fails)
`go build ./...` - проверить, что проект собирается без ошибок
`go test ./...` - run tests
`go test -run TestHealthHandler ./internal/bootstrap` - run specific test: `-run TestName`
`go vet ./...` - Analyzes code for suspicious constructs
`golangci-lint run ./... -c .golangci.yml` - Run linter
 
## Conventions
- Solve the problem, not the consequence
- Consult with me when choosing a library
- Define interfaces where they're used, not where they're implemented
- Use generated mocks with `go.uber.org/mock` (`mockgen`), not handwritten mocks
- For packages under `internal/app/**/adapters/postgres`, write unit tests with DB mocking using `go-mocket` (integration tests are handled separately)
- For new code and new functionality, tests are mandatory with coverage > 80%
- If code generation inputs are changed, run `go generate ./...` and commit generated outputs
- Don't check for nil dependencies that are initialized in New functions (e.g., `NewWorker`)
- Business/worker functions must return structured results (status, retry, error) — never hide outcomes inside logs
- Logging, retries and state transitions must be handled only at orchestration level, not inside leaf functions
- Use structured logging keys only for stable identifiers (e.g. event_id, source_id) and error; all other details must go into the message
- Do not pollute logs with excessive key-value pairs — prefer concise logs with a few meaningful keys and a descriptive message

## Constraints
- Don't touch existing migrations
- Consult with me when choosing a library
