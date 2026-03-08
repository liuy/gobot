# gobot backend Makefile
# Enables FTS5 for go-sqlite3

.PHONY: build test test-race coverage clean

# Build with FTS5 support
build:
	CGO_ENABLED=1 go build -tags "fts5 jieba" ./...

# Run tests with FTS5 support (skip main package which requires frontend)
test:
	CGO_ENABLED=1 go test -tags "fts5 jieba" -v ./channel ./memory ./providers ./log

# Run tests with race detector
test-race:
	CGO_ENABLED=1 go test -tags "fts5 jieba" -race -v ./channel ./memory ./providers ./log

# Run tests with coverage report
coverage:
	CGO_ENABLED=1 go test -tags "fts5 jieba" -coverprofile=coverage.out ./channel ./memory ./providers ./log
	go tool cover -func=coverage.out
	@echo ""
	@echo "HTML report: run 'go tool cover -html=coverage.out'"

# Run all tests including main (requires frontend build)
test-all:
	CGO_ENABLED=1 go test -tags "fts5 jieba" -v ./...

# Clean build artifacts
clean:
	go clean ./...
	rm -f coverage*
