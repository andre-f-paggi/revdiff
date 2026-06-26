# Get the latest commit branch, hash, and date
TAG=$(shell git describe --tags --abbrev=0 --exact-match 2>/dev/null)
BRANCH=$(if $(TAG),$(TAG),$(shell git rev-parse --abbrev-ref HEAD 2>/dev/null))
HASH=$(shell git rev-parse --short=7 HEAD 2>/dev/null)
TIMESTAMP=$(shell git log -1 --format=%ct HEAD 2>/dev/null | xargs -I{} date -u -r {} +%Y%m%dT%H%M%S)
GIT_REV=$(shell printf "%s-%s-%s" "$(BRANCH)" "$(HASH)" "$(TIMESTAMP)")
REV=$(if $(filter --,$(GIT_REV)),latest,$(GIT_REV))
# executable suffix (".exe" on Windows, empty elsewhere) so `make build` emits a
# runnable binary on every platform
GOEXE=$(shell go env GOEXE)

all: test build

build:
	go build -tags forceposix -ldflags "-X main.revision=$(REV) -s -w" -o .bin/revdiff.$(BRANCH)$(GOEXE) ./app
	cp .bin/revdiff.$(BRANCH)$(GOEXE) .bin/revdiff$(GOEXE)

test:
	go clean -testcache
	go test -tags forceposix -race -coverprofile=coverage.out ./...
	grep -v "_mock.go" coverage.out | grep -v mocks > coverage_no_mocks.out
	go tool cover -func=coverage_no_mocks.out
	rm coverage.out coverage_no_mocks.out

lint:
	golangci-lint run

fmt:
	~/.claude/format.sh

race:
	go test -tags forceposix -race -timeout=60s ./...

version:
	@echo "branch: $(BRANCH), hash: $(HASH), timestamp: $(TIMESTAMP)"
	@echo "revision: $(REV)"

site:
	@echo "site assets are in site/ directory"

validate-themes:
	go test -run TestGalleryThemes_validate ./app/theme/

.PHONY: build test lint fmt race version site validate-themes
