VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
GIT_COMMIT ?= $(shell git rev-parse --short=12 HEAD 2>/dev/null || echo unknown)
BUILD_TIME ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

.PHONY: lint
lint:
	golangci-lint run ./...

.PHONY: test
# test: lint
test:
	@echo "Running tests with coverage..."
	@go test ./... -count=1 -coverprofile=coverage.out -covermode=atomic -coverpkg=./...
	@echo "\nGenerating HTML coverage report..."
	@go tool cover -html=coverage.out -o coverage.html
	@echo "HTML coverage report generated: coverage.html"
	@echo "\nTotal Coverage:"
	@go tool cover -func=coverage.out | awk '/^total:/ {print $$3}'

.PHONY: test-ci
# test-ci: no coverage instrumentation (-coverpkg=./... relinks every test binary
# against every package, including the very large vacuum dependency tree) and
# stripped binaries so the ~14 vacuum-linking test binaries link faster.
test-ci:
	go test ./... -ldflags="-s -w"

.PHONY: generate
generate:
	go generate ./...

.PHONY: build
build: generate compile

.PHONY: compile
# compile: build the binary without regenerating code (generated files are committed)
compile:
	go build -ldflags "\
		-X github.com/ifc7/ifc/internal.BuildVersion=$(VERSION) \
		-X github.com/ifc7/ifc/internal.GitCommit=$(GIT_COMMIT) \
		-X github.com/ifc7/ifc/internal.BuildTime=$(BUILD_TIME)" \
		-o ./bin/ifc ./cmd/ifc

.PHONY: install
install: build
	cp ./bin/ifc $(GOPATH)/bin/ifc
