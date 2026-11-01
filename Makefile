BIN      ?= openlist-cli
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT   ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE     ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

# 版本信息通过 -X 注入到 buildinfo，未注入时 buildinfo 会退回模块信息
LDFLAGS := -s -w \
	-X openlist-cli/internal/buildinfo.Version=$(VERSION) \
	-X openlist-cli/internal/buildinfo.Commit=$(COMMIT) \
	-X openlist-cli/internal/buildinfo.Date=$(DATE)

.PHONY: help build test fmt vet lint snapshot cross release clean install

help:
	@echo "build     编译本机二进制到 dist/"
	@echo "test      跑全部单测"
	@echo "fmt       gofmt -w"
	@echo "vet       go vet"
	@echo "cross     手工交叉编译 6 个平台到 dist/"
	@echo "snapshot  本地跑一次 goreleaser 快照（不发布）"
	@echo "release   打 tag 后由 CI 发布，这里只做本地校验"

build:
	CGO_ENABLED=0 go build -trimpath -ldflags '$(LDFLAGS)' -o dist/$(BIN) ./cmd/openlist-cli

test:
	go test ./...

fmt:
	gofmt -w .

vet:
	go vet ./...

cross:
	@mkdir -p dist
	@for target in linux/amd64 linux/arm64 linux/arm darwin/amd64 darwin/arm64 windows/amd64; do \
		os=$${target%/*}; arch=$${target#*/}; \
		out=dist/$(BIN)_$(VERSION)_$${os}_$${arch}; \
		[ "$$os" = windows ] && out=$$out.exe; \
		echo "→ $$os/$$arch"; \
		GOOS=$$os GOARCH=$$arch GOARM=7 CGO_ENABLED=0 \
			go build -trimpath -ldflags '$(LDFLAGS)' -o $$out ./cmd/openlist-cli || exit 1; \
	done
	@ls -lh dist/

snapshot:
	goreleaser release --snapshot --clean

install:
	CGO_ENABLED=0 go install -trimpath -ldflags '$(LDFLAGS)' ./cmd/openlist-cli

clean:
	rm -rf dist
