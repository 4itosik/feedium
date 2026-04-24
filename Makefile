.PHONY: build run lint test test-coverage coverage proto wire generate migrate feediumctl

build:
	go build -o bin/feedium ./cmd/feedium

feediumctl:
	go build -o bin/feediumctl ./cmd/feediumctl

run:
	./bin/feedium -conf configs/

lint:
	golangci-lint run ./... -c .golangci.yml

test:
	go test ./...

test-coverage:
	go test -cover ./...

coverage:
	go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out -o coverage.html && open coverage.html

proto:
	protoc --proto_path=. --proto_path=third_party \
		--go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		--go-http_out=. --go-http_opt=paths=source_relative \
		$$(find internal api -name '*.proto')

wire:
	go run github.com/google/wire/cmd/wire ./...

generate:
	make proto
	make wire

migrate:
	goose -dir migrations postgres "postgres://feesium:feesium@127.0.0.1:5432/feesium?sslmode=disable" up
