BINARY  := escli
PKG     := github.com/bradh11/escli/internal/version
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
GOLANGCI_LINT ?= go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0
LDFLAGS := -s -w -X $(PKG).Version=$(VERSION) -X $(PKG).Commit=$(COMMIT) -X $(PKG).Date=$(DATE)

export CGO_ENABLED := 0

.PHONY: build install test lint check secrets snapshot clean

build: ## Build bin/escli for this machine
	go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/escli

install: ## Install escli into $(go env GOPATH)/bin
	go install -trimpath -ldflags "$(LDFLAGS)" ./cmd/escli

test: ## Run the tests with the race detector
	CGO_ENABLED=1 go test -race ./...

lint: ## Run golangci-lint
	$(GOLANGCI_LINT) run

check: lint test ## Run the linter and the tests

secrets: ## Scan the working tree and git history for secrets (needs gitleaks)
	gitleaks dir . --no-banner --redact
	@if git rev-parse --verify HEAD >/dev/null 2>&1; then gitleaks git --no-banner --redact; fi

snapshot: ## Build release archives for every platform into dist/ without publishing
	goreleaser release --snapshot --clean

clean:
	rm -rf bin dist
