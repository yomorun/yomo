GO ?= go
GOFMT ?= gofmt "-s"
GOFILES := $(shell find . -name "*.go")
VETPACKAGES ?= $(shell $(GO) list ./... | grep -v /examples/)
YOMO_VERSION ?= $(shell git describe --tags 2>/dev/null || git rev-parse --short HEAD)
DATE_FMT = +%Y-%m-%d
ifdef SOURCE_DATE_EPOCH
    BUILD_DATE ?= $(shell date -u -d "@$(SOURCE_DATE_EPOCH)" "$(DATE_FMT)" 2>/dev/null || date -u -r "$(SOURCE_DATE_EPOCH)" "$(DATE_FMT)" 2>/dev/null || date -u "$(DATE_FMT)")
else
    BUILD_DATE ?= $(shell date "$(DATE_FMT)")
endif

GO_LDFLAGS := -X github.com/yomorun/yomo/internal/cmd.Version=$(YOMO_VERSION) $(GO_LDFLAGS)
GO_LDFLAGS := -X github.com/yomorun/yomo/internal/cmd.Date=$(BUILD_DATE) $(GO_LDFLAGS)

.PHONY: fmt
fmt:
	$(GOFMT) -w $(GOFILES)

vet:
	$(GO) vet $(VETPACKAGES)

build:
	$(GO) build -o bin/yomo -ldflags "-s -w ${GO_LDFLAGS}" github.com/yomorun/yomo/cmd/yomo

build-arm:
	GOARCH=arm64 GOOS=linux $(GO) build -o bin/yomo-arm64 -ldflags "-s -w ${GO_LDFLAGS}" github.com/yomorun/yomo/cmd/yomo

build-linux:
	GOOS=linux $(GO) build -o bin/yomo -ldflags "-s -w ${GO_LDFLAGS}" github.com/yomorun/yomo/cmd/yomo

install:
	$(GO) install -ldflags "-s -w ${GO_LDFLAGS}" github.com/yomorun/yomo/cmd/yomo
