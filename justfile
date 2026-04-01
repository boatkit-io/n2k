# list available recipes
default:
    @just --list

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

# tidy go modules
tidy:
    go mod tidy
