BINDIR ?= /usr/local/bin
BINARY ?= vohive
GO_TAGS ?= with_utls nomsgpack
VERSION ?= $(shell git describe --tags --always 2>/dev/null || echo "unknown")
BUILD_TIME ?= $(shell date "+%F %T")
DIST_DIR ?= dist

LDFLAGS = -s -w -buildid= -X 'github.com/iniwex5/vohive/internal/global.Version=$(VERSION)' -X 'github.com/iniwex5/vohive/internal/global.BuildTime=$(BUILD_TIME)'

.PHONY: all build install test clean web

all: build

build: web
	mkdir -p $(DIST_DIR)
	CGO_ENABLED=0 go build -v -trimpath -mod=readonly -buildvcs=false -tags "$(GO_TAGS)" -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(BINARY) ./cmd/vohive

web:
	npm ci --prefix web
	npm run build --prefix web
	rm -rf internal/web/dist
	mkdir -p internal/web
	cp -R web/dist internal/web/dist

install: build
	install -d $(DESTDIR)$(BINDIR)
	install -m 0755 $(DIST_DIR)/$(BINARY) $(DESTDIR)$(BINDIR)

test:
	go vet ./...
	go test ./...

clean:
	go clean
	rm -rf $(DIST_DIR)
	rm -rf web/dist internal/web/dist
