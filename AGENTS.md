See PROJECT.md for project description.
See ARCHITECTURE.md for architecture description.
 
## Stack
Go, Connect-go, GORM, PostgreSQL, slog.
UI: React, Tailwind CSS, go:embed (monorepo)
 
## Key commands
`go run ./cmd/feedium/main.go` - run project
`go test ./...` - run tests
`go test -run TestHealthHandler ./internal/bootstrap` - run specific test: `-run TestName`
`go vet ./...` - Analyzes code for suspicious constructs
 
## Conventions
- Solve the problem, not the consequence
- Consult with me when choosing a library
- Don't touch existing migrations
 
## Constraints
- Don't touch existing migrations
- Consult with me when choosing a library