GO ?= go
GOLANGCI_LINT ?= golangci-lint
GOVULNCHECK ?= govulncheck
PLUGIN_ID := com.github.crypt0rr.default-theme
PLUGIN_VERSION ?= 0.3.0
BUNDLE := dist/$(PLUGIN_ID)-$(PLUGIN_VERSION).tar.gz
COVERAGE_FILE ?= coverage.out

.PHONY: all test coverage security check-style lint build bundle smoke smoke-matrix clean

all: check-style test bundle

test:
	$(GO) test -race ./...

coverage:
	$(GO) test -race -covermode=atomic -coverprofile=$(COVERAGE_FILE) ./...
	$(GO) tool cover -func=$(COVERAGE_FILE)
	@total="$$( $(GO) tool cover -func=$(COVERAGE_FILE) | awk '/^total:/{print $$3}' )"; \
	if [ "$$total" != "100.0%" ]; then \
		echo "Coverage must be 100.0%; got $$total" >&2; \
		exit 1; \
	fi

security: build
	@for binary in server/dist/plugin-*; do \
		$(GOVULNCHECK) -mode=binary "$$binary" || exit 1; \
	done

check-style:
	@test -z "$$(gofmt -l server/*.go)" || (echo "Go files are not formatted"; exit 1)
	$(GO) vet ./...

lint:
	$(GOLANGCI_LINT) run ./...

build:
	rm -rf server/dist
	mkdir -p server/dist
	cd server && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build -trimpath -o dist/plugin-linux-amd64
	cd server && CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build -trimpath -o dist/plugin-linux-arm64
	cd server && CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 $(GO) build -trimpath -o dist/plugin-darwin-amd64
	cd server && CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 $(GO) build -trimpath -o dist/plugin-darwin-arm64
	cd server && CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GO) build -trimpath -o dist/plugin-windows-amd64.exe

bundle: build
	rm -rf dist
	mkdir -p dist/$(PLUGIN_ID)/server/dist
	sed 's/"version": "[^"]*"/"version": "$(PLUGIN_VERSION)"/' plugin.json > dist/$(PLUGIN_ID)/plugin.json
	cp -r assets dist/$(PLUGIN_ID)/
	cp server/dist/* dist/$(PLUGIN_ID)/server/dist/
	tar -czf $(BUNDLE) -C dist $(PLUGIN_ID)
	@echo "Plugin bundle: $(BUNDLE)"

smoke: bundle
	bash scripts/smoke-test.sh

smoke-matrix: bundle
	bash scripts/docker-smoke-matrix.sh

clean:
	rm -rf dist server/dist
