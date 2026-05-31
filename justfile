# Mimi Build System
# Version information (can be overridden)

VERSION := `git describe --tags --always --dirty 2>/dev/null || echo "dev"`
GIT_COMMIT := `git rev-parse --short HEAD 2>/dev/null || echo "unknown"`
BUILD_DATE := `date -u +"%Y-%m-%dT%H:%M:%SZ"`

# Ldflags for version injection

LDFLAGS := "-s -w -X github.com/y3owk1n/mimi/cmd/mimi/cmd.Version=" + VERSION + " -X github.com/y3owk1n/mimi/cmd/mimi/cmd.GitCommit=" + GIT_COMMIT + " -X github.com/y3owk1n/mimi/cmd/mimi/cmd.BuildDate=" + BUILD_DATE

# Default build
default: build

# Build the binary
build:
    @echo "Building Mimi..."
    @echo "Version: {{ VERSION }}"
    {{ if os() == "windows" { "CGO_ENABLED=0" } else { "CGO_ENABLED=1" } }} go build -ldflags="{{ LDFLAGS }}" -o bin/mimi{{ if os() == "windows" { ".exe" } else { "" } }} ./cmd/mimi
    @echo "✓ Build complete: bin/mimi"

build-darwin:
    @echo "Building Mimi for macOS..."
    mkdir -p bin
    CGO_ENABLED=1 go build -ldflags="{{ LDFLAGS }}" -o bin/mimi-darwin ./cmd/mimi
    @echo "✓ Build complete: bin/mimi-darwin"

release:
    @echo "Building release version..."
    @echo "Version: {{ VERSION }}"
    @echo "Commit: {{ GIT_COMMIT }}"
    @echo "Date: {{ BUILD_DATE }}"
    CGO_ENABLED=1 go build -ldflags="{{ LDFLAGS }}" -trimpath -o bin/mimi ./cmd/mimi
    @echo "✓ Release build complete: bin/mimi"

# Usage: just release-ci-darwin arm64 v1.2.3
release-ci-darwin ARCH VERSION_OVERRIDE:
    @echo "Building release artifact (darwin/{{ ARCH }}) for CI..."
    @echo "Version: {{ VERSION_OVERRIDE }}"
    @echo "Commit: {{ GIT_COMMIT }}"
    @echo "Date: {{ BUILD_DATE }}"
    mkdir -p bin
    CGO_ENABLED=1 GOOS=darwin GOARCH={{ ARCH }} go build -ldflags="-s -w -X github.com/y3owk1n/mimi/cmd/mimi/cmd.Version={{ VERSION_OVERRIDE }} -X github.com/y3owk1n/mimi/cmd/mimi/cmd.GitCommit={{ GIT_COMMIT }} -X github.com/y3owk1n/mimi/cmd/mimi/cmd.BuildDate={{ BUILD_DATE }}" -trimpath -o bin/mimi-darwin-{{ ARCH }} ./cmd/mimi
    @echo "✓ Release artifact for darwin/{{ ARCH }} built successfully"

# Bundle the application
bundle: release
    @echo "Bundling Mimi..."
    mkdir -p build/Mimi.app/Contents/{MacOS,Resources}

    cp -r bin/mimi build/Mimi.app/Contents/MacOS/mimi

    # cp resources/icon.icns build/Mimi.app/Contents/Resources/icon.icns

    sed "s/VERSION/{{ VERSION }}/g" resources/Info.plist.template > build/Mimi.app/Contents/Info.plist

    codesign --force --deep --sign - build/Mimi.app

    @echo "✓ Bundle complete: build/Mimi.app"

# Run tests

# Run all tests (unit + integration)
test: test-unit test-integration
    @echo "Running all tests..."

# Run unit tests
test-unit:
    @echo "Running unit tests..."
    go test -v ./...

test-integration:
    @echo "Running integration tests..."
    go test -tags=integration -v ./...

test-race: test-race-unit test-race-integration
    @echo "Running tests with race detection..."

test-race-unit:
    @echo "Running unit tests with race detection..."
    go test -race -v ./...

# Run integration tests with race detection
test-race-integration:
    @echo "Running integration tests with race detection..."
    go test -tags=integration -race -v ./...

test-all: test test-race

fmt-check:
    #!/usr/bin/env bash
    echo "Not checking formatting for go files... It will be checked in lint"
    echo "Checking Objective-C file formatting..."
    EXIT_CODE=0
    while IFS= read -r -d '' file; do
        case "$file" in *.c) af=file.c;; *) af=file.m;; esac
        OUTPUT=$(clang-format --dry-run -Werror --style=file --assume-filename="$af" "$file" 2>&1)
        RESULT=$?
        # Filter out the "does not support C++" warnings
        FILTERED=$(echo "$OUTPUT" | grep -v "Configuration file(s) do(es) not support C++")
        if [ -n "$FILTERED" ]; then
            echo "$FILTERED"
        fi
        if [ $RESULT -ne 0 ] && [ -n "$FILTERED" ]; then
            EXIT_CODE=1
        fi
    done < <(find internal/observers/cgo_bridge \( -name "*.h" -o -name "*.m" -o -name "*.c" \) -print0)
    if [ $EXIT_CODE -ne 0 ]; then
        echo "Some Objective-C files are not properly formatted. Run 'just fmt' to fix them."
        exit 1
    fi
    echo "✓ All Objective-C files are properly formatted"

clean:
    @echo "Cleaning build artifacts..."
    rm -rf bin/
    rm -rf build/
    rm -rf *.app
    @echo "✓ Clean complete"

# Format code
fmt:
    @echo "Formatting Go files..."
    golangci-lint fmt
    golangci-lint run --fix
    @echo "Formatting Objective-C files..."
    @find internal/observers/cgo_bridge \( -name "*.h" -o -name "*.m" -o -name "*.c" \) -exec sh -c 'case "$1" in *.c) af=file.c;; *) af=file.m;; esac; clang-format -i --style=file --assume-filename="$af" "$1"' _ {} \;
    @echo "✓ Format complete"

# Lint code
lint:
    @echo "Linting code..."
    golangci-lint run
    @echo "Linting Objective-C files..."
    echo "Skipping Objective-C linting due to header issues"
    @echo "✓ Lint complete"

# Vet
vet:
    @echo "Vetting code..."
    go vet ./...
    @echo "✓ Vet complete"

# Download dependencies
deps:
    @echo "Downloading dependencies..."
    go mod download
    go mod tidy
    @echo "✓ Dependencies updated"

# Verify dependencies
verify:
    @echo "Verifying dependencies..."
    go mod verify
    @echo "✓ Dependencies verified"
