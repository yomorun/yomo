GO ?= go
GOFMT ?= gofmt "-s"
GOLINT ?= golint
GOFILES := $(shell find . -name "*.go")
VETPACKAGES ?= $(shell $(GO) list ./... | grep -v example)

.PHONY: fmt
fmt:
	$(GOFMT) -w $(GOFILES)

vet:
	$(GO) vet $(VETPACKAGES)

lint:
	$(GOLINT) $(GOFILES)

test:
	$(GO) test $(VETPACKAGES)

cover:
	$(GO) test $(VETPACKAGES) -coverprofile=prof.out && $(GO) tool cover -html=prof.out && rm prof.out