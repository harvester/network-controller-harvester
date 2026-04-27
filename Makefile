ROOT := $(shell dirname $(realpath $(lastword $(MAKEFILE_LIST))))
MK_HOST_ARCH := $(shell uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
export MK_HOST_ARCH

DOCKER_BUILDKIT := 1
export DOCKER_BUILDKIT

MK_DOCKER_PROGRESS ?= plain
export MK_DOCKER_PROGRESS

MK_REPO_ID := $(shell echo -n "$(ROOT)$$(cat /etc/machine-id 2>/dev/null)" | sha256sum | cut -c1-8)
export MK_REPO_ID

ifdef CI
  BOLD  :=
  CYAN  :=
  RESET :=
else
  BOLD  := \033[1m
  CYAN  := \033[36m
  RESET := \033[0m
endif

BANNER = @printf "$(BOLD)$(CYAN)[target: $@]$(RESET)\n"

DOCKER_BUILD = docker build \
    --progress=$(MK_DOCKER_PROGRESS) \
    --build-arg MK_REPO_ID \
    --build-arg MK_HOST_ARCH \
    -f $(ROOT)/Dockerfile $(ROOT)

.DEFAULT_GOAL := ci

.PHONY: build ci gen-version-env test validate package generate generate-manifest clean clean-all

# ---- gen-version-env ----
gen-version-env:
	@bash $(ROOT)/scripts/version > /dev/null

# ---- build ----
build: gen-version-env
	$(BANNER)
	$(DOCKER_BUILD) --target build-output \
	    --output type=local,dest=$(ROOT)

# ---- test ----
test: gen-version-env
	$(BANNER)
	$(DOCKER_BUILD) --target test

# ---- validate ----
validate: gen-version-env
	$(BANNER)
	$(DOCKER_BUILD) --target validate

# ---- package ----
package: build
	$(BANNER)
	ARCH=$(MK_HOST_ARCH) $(ROOT)/scripts/package

# ---- generate ----
generate: gen-version-env
	$(BANNER)
	$(DOCKER_BUILD) --target generate-output \
	    --output type=local,dest=$(ROOT)

# ---- generate-manifest ----
generate-manifest: gen-version-env
	$(BANNER)
	$(DOCKER_BUILD) --target generate-manifest-output \
	    --output type=local,dest=$(ROOT)

# ---- ci ----
ci: build test validate package

# ---- clean ----
clean:
	@rm -rf $(ROOT)/bin

clean-all: clean
