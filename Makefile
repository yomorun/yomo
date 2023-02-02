GO ?= go
GOFMT ?= gofmt "-s"
GOFILES := $(shell find . -name "*.go")
VETPACKAGES ?= $(shell $(GO) list ./... | grep -v /example/)

.PHONY: fmt
fmt:
	$(GOFMT) -w $(GOFILES)

vet:
	$(GO) vet $(VETPACKAGES)

lint:
	revive -exclude example/... -exclude cli/... -formatter friendly ./...

build:
	$(GO) build -tags "$(TAGS)" -o bin/yomo -ldflags "-s -w ${GO_LDFLAGS}" ./cmd/yomo/main.go

unittest:
	$(GO) test -v -race -covermode=atomic $(go list ./... | grep -v /example)