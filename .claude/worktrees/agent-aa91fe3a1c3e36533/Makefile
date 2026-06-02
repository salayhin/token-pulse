.PHONY: build run index stats test lint clean tidy snapshot release-dry

BIN := bin/tokenpulse

build:
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o $(BIN) ./cmd/tokenpulse

run:
	go run ./cmd/tokenpulse serve

index:
	go run ./cmd/tokenpulse index

stats:
	go run ./cmd/tokenpulse stats

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
