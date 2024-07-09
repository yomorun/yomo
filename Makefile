GO ?= go
GOFMT ?= gofmt "-s"
GOFILES := $(shell find . -name "*.go")
VETPACKAGES ?= $(shell $(GO) list ./... | grep -v /example/)
TAGS ?= $(shell git describe --tags 2>/dev/null || git rev-parse --short HEAD)

.PHONY: fmt
fmt:
	$(GOFMT) -w $(GOFILES)

.PHONY: vet
vet:
	$(GO) vet $(VETPACKAGES)

.PHONY: lint
lint:
	revive -exclude example/... -exclude cli/... -exclude vendor/... -exclude TEST -formatter friendly ./...

.PHONY: build
build:
	$(GO) build -race -tags "$(TAGS)" -o bin/yomo -trimpath -ldflags "-s -w" ./cmd/yomo/main.go

.PHONY: test
test:
	$(GO) test -race -covermode=atomic $(VETPACKAGES)

.PHONY: coverage
coverage:
	$(GO) test -v -race -coverprofile=coverage.txt -covermode=atomic $(VETPACKAGES)
