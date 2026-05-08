.PHONY: build run index stats test lint clean tidy snapshot release-dry

BIN := bin/claude-token-lens

build:
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o $(BIN) ./cmd/claude-token-lens

run:
	go run ./cmd/claude-token-lens serve

index:
	go run ./cmd/claude-token-lens index

stats:
	go run ./cmd/claude-token-lens stats

test:
	go test ./... -race -count=1

lint:
	golangci-lint run

tidy:
	go mod tidy

snapshot:
	goreleaser release --snapshot --clean

release-dry:
	goreleaser release --clean --skip=publish

clean:
	rm -rf bin/ dist/
