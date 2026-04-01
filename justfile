# list available recipes
default:
    @just --list

# build all binaries locally
build:
    go build -v -o bin/ ./cmd/...

# run all tests
test:
    go test ./...

# run tests with verbose output
test-v:
    go test -v ./...

# run tests with race detector
test-race:
    go test -race ./...

# run tests with coverage
test-cover:
    go test -coverpkg=./... -coverprofile=c.out ./...

# format all Go source files
fmt:
    gofmt -w .

# run go vet
vet:
    go vet ./...

# run golangci-lint
lint:
    golangci-lint run ./...

# run code generation
codegen:
    go run ./cmd/pgngen/main.go ./cmd/pgngen/deduper.go

# remove build artifacts
clean:
    rm -rf bin
    go clean ./...

# tidy go modules
tidy:
    go mod tidy
