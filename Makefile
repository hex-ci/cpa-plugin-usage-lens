PLUGIN := usage-lens.so

.PHONY: build build-panel test lint clean release

GO ?= go
VERSION ?= $(shell cat VERSION 2>/dev/null || echo dev)
LDFLAGS := -X main.version=$(VERSION)

# Default target: build the plugin for the current platform.
build: build-panel
	CGO_ENABLED=1 $(GO) build -buildmode=c-shared -ldflags "$(LDFLAGS)" -o $(PLUGIN) .

build-panel:
	cd panel && pnpm install && pnpm build

test:
	$(GO) test -race -count=1 ./...

lint:
	@test -z "$$($(GO)fmt -l .)" || ($(GO)fmt -l . && exit 1)
	$(GO) vet ./...

clean:
	rm -f $(PLUGIN) usage-lens.h
	rm -rf dist/

# Cross-compile for all supported platforms (requires cross C toolchains;
# this machine usually only produces linux/amd64, others are skipped).
# Artifacts: dist/usage-lens_<ver>_<os>_<arch>.zip (zip root holds the .so
# directly) + dist/checksums.txt (sha256 over every zip).
release: clean build-panel
	@mkdir -p dist
	@for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64; do \
	  os=$${target%/*}; arch=$${target#*/}; \
	  name=usage-lens_$(VERSION)_$${os}_$${arch}; \
	  mkdir -p dist/$$name; \
	  if GOOS=$$os GOARCH=$$arch CGO_ENABLED=1 $(GO) build -buildmode=c-shared -ldflags "$(LDFLAGS)" -o dist/$$name/$(PLUGIN) . 2>/tmp/usage-lens-cross.log; then \
	    cd dist/$$name && zip -qr ../$$name.zip $(PLUGIN) && cd .. && rm -rf $$name; \
	    echo "built $$name"; \
	  else \
	    echo "skip $$target (cross toolchain unavailable)"; \
	    rm -rf dist/$$name; \
	  fi; \
	done
	@cd dist && sha256sum *.zip | tee checksums.txt
	@echo "--- checksums:"; cat dist/checksums.txt