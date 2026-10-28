# SPDX-License-Identifier: Apache-2.0
# SPDX-FileCopyrightText: 2026 The semrel Authors

# ── build stage ────────────────────────────────────────────────────────────────
FROM --platform=$BUILDPLATFORM debian:13-slim@sha256:89c1ea91e897f0fe0a3731595330bc45b8bdef4047464a07116f891a94054b31 AS build

RUN apt-get update && \
    apt-get -y --no-install-recommends install ca-certificates curl git build-essential && \
    rm -rf /var/lib/apt/lists/*

SHELL ["/bin/bash", "-o", "pipefail", "-c"]
ENV PATH="/root/.local/bin:/mise/shims:$PATH" \
    MISE_DATA_DIR="/mise" \
    MISE_CONFIG_DIR="/mise" \
    MISE_CACHE_DIR="/mise/cache" \
    MISE_INSTALL_PATH="/usr/local/bin/mise"

RUN curl --proto '=https' --proto-redir '=https' \
    --fail --show-error --silent --location https://mise.run | sh

COPY mise.toml ./
RUN --mount=type=cache,target=/mise/cache \
   mise trust && mise install

WORKDIR /src

COPY go.mod go.sum* ./
RUN --mount=type=cache,target=/root/go/pkg \
    go mod download -x

COPY . .

ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev

RUN GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    task build

# ── distroless release image ───────────────────────────────────────────────────
FROM gcr.io/distroless/static-debian13:nonroot@sha256:18f4cc26c9ffa7f20a359298be7deadac5c607c8e03a02b8119f1a5fc2f82c99

COPY --from=build /src/bin/plugin /usr/local/bin/plugin
USER nonroot
ENTRYPOINT ["/usr/local/bin/plugin"]
