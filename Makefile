.PHONY: build install clean test

BINARY := dp
BUILD_DIR := .
INSTALL_DIR := $(HOME)/.local/bin

# Version info for ldflags
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

LDFLAGS := -X main.version=$(VERSION)

# ICU4C detection for macOS (required by go-icu-regex transitive dependency).
# Homebrew installs icu4c as a keg-only package, so headers/libs aren't on the
# default search path. Auto-detect the prefix and export CGo flags.
ifeq ($(shell uname),Darwin)
  ICU_PREFIX := $(shell brew --prefix icu4c 2>/dev/null)
  ifneq ($(ICU_PREFIX),)
    export CGO_CPPFLAGS += -I$(ICU_PREFIX)/include
    export CGO_LDFLAGS  += -L$(ICU_PREFIX)/lib
  endif
endif

build:
	go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY) ./cmd/dp
ifeq ($(shell uname),Darwin)
	@codesign -s - -f $(BUILD_DIR)/$(BINARY) 2>/dev/null || true
	@echo "Signed $(BINARY) for macOS"
endif

install: build
	@mkdir -p $(INSTALL_DIR)
	@rm -f $(INSTALL_DIR)/$(BINARY)
	@cp $(BUILD_DIR)/$(BINARY) $(INSTALL_DIR)/$(BINARY)
	@# Remove stale go-install binaries that shadow the canonical location
	@for bad in $(HOME)/go/bin/$(BINARY) $(HOME)/bin/$(BINARY); do \
		if [ -f "$$bad" ]; then \
			echo "Removing stale $$bad (use make install, not go install)"; \
			rm -f "$$bad"; \
		fi; \
	done
	@echo "Installed $(BINARY) to $(INSTALL_DIR)/$(BINARY)"

clean:
	rm -f $(BUILD_DIR)/$(BINARY)

test:
	go test ./...
