.PHONY: build test lint cover clean run install bench-locomo sidecar-install sidecar-start

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

docker:
	docker build -t hydra:latest .

docker-run: docker
	docker run -p 8080:8080 -v $(PWD)/configs:/app/configs hydra:latest

bench-locomo:
	go run ./cmd/bench-locomo $(if $(STRATEGY),--strategy $(STRATEGY)) $(if $(LIMIT),--limit $(LIMIT))

sidecar-install:
	cd tools/llm-sidecar && npm install

sidecar-start:
	cd tools/llm-sidecar && npm start

install:
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
	go install golang.org/x/tools/cmd/goimports@latest
	go install github.com/evilmartians/lefthook@latest
	lefthook install
