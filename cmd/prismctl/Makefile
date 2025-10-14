.PHONY: build test clean install lint

# Build binary
build:
	go build -o prismctl .

# Build with version info
build-release:
	go build -ldflags="-X 'main.Version=$(shell git describe --tags --always --dirty)'" -o prismctl .

# Run tests
test:
	go test ./... -v -race -coverprofile=coverage.out

# Run tests with coverage report
test-coverage:
	go test ./... -race -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report written to coverage.html"

# Clean build artifacts
clean:
	rm -f prismctl prismctl-bin coverage.out coverage.html

# Install binary to $GOPATH/bin
install:
	go install .

# Run linter
lint:
	golangci-lint run ./...

# Format code
fmt:
	go fmt ./...
	goimports -w .

# Run go mod tidy
tidy:
	go mod tidy

# Development build and run
run:
	go run . --help
