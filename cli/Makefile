GO ?= go
GOFMT ?= gofmt "-s"
GOFILES := $(shell find . -name "*.go")
VETPACKAGES ?= $(shell $(GO) list ./... | grep -v /examples/)
CLI_VERSION ?= $(shell git describe --tags 2>/dev/null || git rev-parse --short HEAD)
GO_LDFLAGS ?= -X $(shell $(GO) list -m)/cli.Version=$(CLI_VERSION)
VER ?= $(shell git describe --tags --abbrev=0)
TAGS ?=

bina_json = '{"platforms": { "darwin-arm64": { "asset": "yomo-${VER}-arm64-Darwin.tar.gz", "file": "yomo" }, "darwin-amd64": { "asset": "yomo-${VER}-x86_64-Darwin.tar.gz", "file": "yomo" }, "linux-arm64": { "asset": "yomo-${VER}-arm64-Linux.tar.gz", "file": "yomo" }, "linux-amd64": { "asset": "yomo-${VER}-x86_64-Linux.tar.gz", "file": "yomo" }, "windows-amd64": { "asset": "yomo-${VER}-x86_64-Windows.tar.gz", "file": "yomo.exe" } } }'

.PHONY: fmt
fmt:
	$(GOFMT) -w $(GOFILES)

vet:
	$(GO) vet $(VETPACKAGES)

build:
	$(GO) build -tags "$(TAGS)" -o bin/yomo -ldflags "-s -w ${GO_LDFLAGS}" ../cmd/yomo/main.go

archive-release:
	rm -rf bin/yomo
	GOARCH=arm64 GOOS=darwin $(GO) build -tags "$(TAGS)" -o bin/yomo -ldflags "-s -w ${GO_LDFLAGS}" ../cmd/yomo/main.go
	tar -C ./bin -czf bin/yomo-${VER}-arm64-Darwin.tar.gz yomo
	rm -rf bin/yomo
	GOARCH=amd64 GOOS=darwin $(GO) build -tags "$(TAGS)" -o bin/yomo -ldflags "-s -w ${GO_LDFLAGS}" ../cmd/yomo/main.go
	tar -C ./bin -czf bin/yomo-${VER}-x86_64-Darwin.tar.gz yomo
	rm -rf bin/yomo
	GOARCH=arm64 GOOS=linux $(GO) build -tags "$(TAGS)" -o bin/yomo -ldflags "-s -w ${GO_LDFLAGS}" ../cmd/yomo/main.go
	tar -C ./bin -czf bin/yomo-${VER}-arm64-Linux.tar.gz yomo
	rm -rf bin/yomo
	GOARCH=amd64 GOOS=linux $(GO) build -tags "$(TAGS)" -o bin/yomo -ldflags "-s -w ${GO_LDFLAGS}" ../cmd/yomo/main.go
	tar -C ./bin -czf bin/yomo-${VER}-x86_64-Linux.tar.gz yomo
	rm -rf bin/yomo
	GOARCH=amd64 GOOS=windows $(GO) build -tags "$(TAGS)" -o bin/yomo.exe -ldflags "-s -w ${GO_LDFLAGS}" ../cmd/yomo/main.go
	tar -C ./bin -czf bin/yomo-${VER}-x86_64-Windows.tar.gz yomo.exe
	rm -rf bin/yomo.exe
	make bina

archive-release-linux-amd64:
	rm -rf bin/yomo
	GOARCH=amd64 GOOS=linux $(GO) build -o bin/yomo -ldflags "-s -w ${GO_LDFLAGS}" ../cmd/yomo/main.go
	tar -C ./bin -czf bin/yomo-${VER}-x86_64-Linux.tar.gz yomo

archive-release-linux-arm64:
	rm -rf bin/yomo
	GOARCH=arm64 GOOS=linux $(GO) build -o bin/yomo -ldflags "-s -w ${GO_LDFLAGS}" ../cmd/yomo/main.go
	tar -C ./bin -czf bin/yomo-${VER}-arm64-Linux.tar.gz yomo

archive-release-darwin-amd64:
	rm -rf bin/yomo
	GOARCH=amd64 GOOS=darwin $(GO) build -o bin/yomo -ldflags "-s -w ${GO_LDFLAGS}" ../cmd/yomo/main.go
	tar -C ./bin -czf bin/yomo-${VER}-x86_64-Darwin.tar.gz yomo

archive-release-darwin-arm64:
	rm -rf bin/yomo
	GOARCH=arm64 GOOS=darwin $(GO) build -o bin/yomo -ldflags "-s -w ${GO_LDFLAGS}" ../cmd/yomo/main.go
	tar -C ./bin -czf bin/yomo-${VER}-arm64-Darwin.tar.gz yomo

archive-release-windows-amd64:
	$(GO) build -o bin/yomo.exe -ldflags "-s -w ${GO_LDFLAGS}" ../cmd/yomo/main.go
	tar -C ./bin -czf bin/yomo-${VER}-x86_64-Windows.tar.gz yomo.exe

build-w-sym:
	GOARCH=amd64 GOOS=linux $(GO) build -o bin/yomo -ldflags "${GO_LDFLAGS}" -gcflags=-l ../cmd/yomo/main.go

bina:
	mkdir -p bin
	rm -rf bin/bina.json
	@echo ${bina_json} > bin/bina.json

cli_version:
	@echo ${VER} > ./VERSION
