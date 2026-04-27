# syntax=docker/dockerfile:1
# check=skip=InvalidDefaultArgInFrom

FROM registry.suse.com/bci/golang:1.25.7 AS builder
ARG MK_HOST_ARCH
ENV ARCH=$MK_HOST_ARCH
RUN zypper -n rm container-suseconnect && \
    zypper -n install git curl docker gzip tar wget awk
COPY --from=golangci/golangci-lint:v2.11.4-alpine@sha256:72bcd68512b4e27540dd3a778a1b7afd45759d8145cfb3c089f1d7af53e718e9 \
    /usr/bin/golangci-lint /usr/local/bin/golangci-lint
RUN GO111MODULE=on go install sigs.k8s.io/controller-tools/cmd/controller-gen@v0.18.0 && \
    GO111MODULE=on go install golang.org/x/tools/cmd/goimports@v0.43.0
WORKDIR /go/src/github.com/harvester/harvester-network-controller
ENV HOME=/go/src/github.com/harvester/harvester-network-controller

# ---- base ----
FROM builder AS base
WORKDIR /go/src/github.com/harvester/harvester-network-controller
COPY . .

# ---- build ----
FROM base AS build
ARG MK_REPO_ID
RUN --mount=type=cache,target=/go/pkg/mod,id=harvester-network-controller-go-mod-${MK_REPO_ID} \
    --mount=type=cache,target=/go/src/github.com/harvester/harvester-network-controller/.cache/go-build,id=harvester-network-controller-go-build-${MK_REPO_ID} \
    ./scripts/build

# ---- build-output ----
FROM scratch AS build-output
COPY --from=build /go/src/github.com/harvester/harvester-network-controller/bin/ /bin/

# ---- test ----
FROM base AS test
ARG MK_REPO_ID
RUN --mount=type=cache,target=/go/pkg/mod,id=harvester-network-controller-go-mod-${MK_REPO_ID} \
    --mount=type=cache,target=/go/src/github.com/harvester/harvester-network-controller/.cache/go-build,id=harvester-network-controller-go-build-${MK_REPO_ID} \
    ./scripts/test

# ---- validate ----
FROM base AS validate
ARG MK_REPO_ID
RUN --mount=type=cache,target=/go/pkg/mod,id=harvester-network-controller-go-mod-${MK_REPO_ID} \
    --mount=type=cache,target=/go/src/github.com/harvester/harvester-network-controller/.cache/go-build,id=harvester-network-controller-go-build-${MK_REPO_ID} \
    ./scripts/validate

# ---- generate ----
FROM base AS generate
ARG MK_REPO_ID
RUN --mount=type=cache,target=/go/pkg/mod,id=harvester-network-controller-go-mod-${MK_REPO_ID} \
    --mount=type=cache,target=/go/src/github.com/harvester/harvester-network-controller/.cache/go-build,id=harvester-network-controller-go-build-${MK_REPO_ID} \
    ./scripts/generate

# ---- generate-output ----
FROM scratch AS generate-output
COPY --from=generate /go/src/github.com/harvester/harvester-network-controller/pkg/ /pkg/

# ---- generate-manifest ----
FROM base AS generate-manifest
RUN ./scripts/generate-manifest

# ---- generate-manifest-output ----
FROM scratch AS generate-manifest-output
COPY --from=generate-manifest /go/src/github.com/harvester/harvester-network-controller/manifests/ /manifests/
