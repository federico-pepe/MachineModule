BINARY    := machinemodule
REPO_ROOT := $(abspath $(CURDIR))
# audiobus is not in a tagged core yet: build against a local core/ checkout.
# Drop CORE and the MODFILE lines once core/v* has audiobus and go.mod pins it.
CORE      ?= $(HOME)/Documents/GitHub/ableton-push-hack/core
DOCKER    := docker run --rm --platform linux/amd64 \
               -v "$(REPO_ROOT)":/work -v "$(CORE)":/core -w /work/src \
               golang:1.25-bookworm
MODFILE   := cp go.mod /tmp/m.mod && cp go.sum /tmp/m.sum && \
               go mod edit -modfile=/tmp/m.mod -replace github.com/federico-pepe/ableton-push-hack/core=/core && \
               go mod tidy -modfile=/tmp/m.mod
GOFLAGS   := -modfile=/tmp/m.mod

.PHONY: all build test clean

all: build

# cgo (libasound) means a plain `go build` cannot cross-compile; build inside a
# linux/amd64 container. AbletonOS keeps its dynamic linker at
# /lib/ld-linux-x86-64.so.2, not /lib64/, which the extldflags fix.
build:
	@echo "Building $(BINARY) (cgo, linux/amd64, via Docker)..."
	$(DOCKER) sh -c "apt-get update -qq && apt-get install -qq -y libasound2-dev >/dev/null && \
	  $(MODFILE) && \
	  CGO_ENABLED=1 go build $(GOFLAGS) -ldflags '-s -w -linkmode external -extldflags -Wl,--dynamic-linker=/lib/ld-linux-x86-64.so.2' -o ../$(BINARY) . && \
	  chmod +x ../$(BINARY)"
	@echo "Done: $(BINARY)"

# The engine, sequencer, panel and storage are plain Go and test anywhere
# (no Push, no Docker needed): `cd src && go test -race ./...`
test:
	$(DOCKER) sh -c "apt-get update -qq && apt-get install -qq -y libasound2-dev >/dev/null && \
	  $(MODFILE) && go vet $(GOFLAGS) ./... && go test $(GOFLAGS) -race ./..."

clean:
	rm -f $(BINARY)
