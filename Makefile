.PHONY: build run lint test proto wire generate

build:
	go build -o bin/feedium ./cmd/feedium

run:
	./bin/feedium -conf configs/

lint:
	golangci-lint run ./... -c .golangci.yml

test:
	go test ./...

proto:
	protoc --proto_path=. --go_out=. --go_opt=paths=source_relative $$(find internal -name '*.proto')

wire:
	go run github.com/google/wire/cmd/wire ./...

generate:
	make proto
	make wire
