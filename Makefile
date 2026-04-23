.PHONY: build test lint cover clean run

build:
	go build -o bin/hydra ./cmd/hydra

test:
	go test -v -race ./...

lint:
	golangci-lint run ./...

cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

clean:
	rm -rf bin/ coverage.out coverage.html

run: build
	./bin/hydra
