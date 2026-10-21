BINDIR ?= /usr/local/bin
BINARY ?= vohive
GO_TAGS ?= with_utls nomsgpack
VERSION ?= $(shell git describe --tags --always 2>/dev/null || echo "unknown")
BUILD_TIME ?= $(shell date "+%F %T")
DIST_DIR ?= dist
WEB_DEPS_STAMP := web/node_modules/.vohive-deps
WEB_BUILD_STAMP := .cache/web-build
WEB_INPUTS := Makefile web \
	$(shell find web/src web/public -type f -o -type d) \
	$(wildcard web/*.json web/*.ts web/*.js web/*.cjs web/*.mjs web/.env* web/.npmrc)
WEB_OUTPUTS := web/dist/index.html internal/web/dist/index.html

LDFLAGS = -s -w -buildid= -X 'github.com/iniwex5/vohive/internal/global.Version=$(VERSION)' -X 'github.com/iniwex5/vohive/internal/global.BuildTime=$(BUILD_TIME)'

.PHONY: all build install test clean web FORCE

all: build

build: web
	mkdir -p $(DIST_DIR)
	CGO_ENABLED=0 go build -v -trimpath -mod=readonly -buildvcs=false -tags "$(GO_TAGS)" -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(BINARY) ./cmd/vohive

web: $(WEB_BUILD_STAMP)

$(WEB_DEPS_STAMP): web/package.json web/package-lock.json $(wildcard web/.npmrc)
	npm ci --prefix web
	touch $@

$(WEB_BUILD_STAMP): $(WEB_INPUTS) $(WEB_DEPS_STAMP) $(if $(filter-out $(wildcard $(WEB_OUTPUTS)),$(WEB_OUTPUTS)),FORCE)
	npm run build --prefix web
	rm -rf internal/web/dist
	mkdir -p internal/web
	cp -R web/dist internal/web/dist
	mkdir -p $(dir $@)
	touch $@

FORCE:

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
	rm -f $(WEB_DEPS_STAMP) $(WEB_BUILD_STAMP)
