set shell := ["bash", "-uc"]

binary := "mimi"
version := `git describe --tags --always --dirty 2>/dev/null || echo "dev"`
ldflags := "-X github.com/y3owk1n/mimi/cmd/mimi/cmd.version=" + version
build_flags := "-ldflags \"" + ldflags + "\""

# Build the binary
build:
    CGO_ENABLED=1 go build {{build_flags}} -o ./bin/{{binary}} ./cmd/mimi

# Build universal binary (arm64 + amd64)
build-universal:
    CGO_ENABLED=1 GOARCH=arm64 go build {{build_flags}} -o ./bin/{{binary}}-arm64 ./cmd/mimi
    CGO_ENABLED=1 GOARCH=amd64 go build {{build_flags}} -o ./bin/{{binary}}-amd64 ./cmd/mimi
    lipo -create -output ./bin/{{binary}} ./bin/{{binary}}-arm64 ./bin/{{binary}}-amd64
    rm ./bin/{{binary}}-arm64 ./bin/{{binary}}-amd64

# Code sign the binary
sign: build
    codesign --force --sign - ./bin/{{binary}}

# Build and sign (development)
dev: build sign

# Run tests
test:
    go test ./internal/... -count=1

# Run tests with race detector
test-race:
    go test -race ./internal/... -count=1

# Run linter
lint:
    golangci-lint run ./...

# Tidy dependencies
tidy:
    go mod tidy

# Install the binary and hooks
install: dev
    cp ./bin/{{binary}} /usr/local/bin/{{binary}}
    {{binary}} install

# Uninstall the binary and hooks
uninstall:
    {{binary}} uninstall || true
    rm -f /usr/local/bin/{{binary}}

# Clean build artifacts
clean:
    rm -rf ./bin
