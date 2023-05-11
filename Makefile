GO ?= go
GOFMT ?= gofmt "-s"
GOFILES := $(shell find . -name "*.go")
VETPACKAGES ?= $(shell $(GO) list ./... | grep -v /example/)

.PHONY: fmt
fmt:
	$(GOFMT) -w $(GOFILES)

.PHONY: vet
vet:
	$(GO) vet $(VETPACKAGES)

.PHONY: lint
lint:
	revive -exclude example/... -exclude cli/... -formatter friendly ./...

.PHONY: build
build:
	$(GO) build -tags "$(TAGS)" -o bin/yomo -trimpath -ldflags "-s -w" ./cmd/yomo/main.go

.PHONY: test
test:
	$(GO) test -race -covermode=atomic $(go list ./... | grep -v /example)

.PHONY: coverage
coverage:
	$(GO) test -v -race -coverprofile=coverage.txt -covermode=atomic $(VETPACKAGES)
