# syntax=docker/dockerfile:1

FROM golang:1.27.3-bookworm AS go-toolchain
FROM node:22-bookworm-slim AS node-toolchain

# Noble is Ubuntu 24.04, matching the GitHub Actions release runner.
FROM mcr.microsoft.com/dotnet/sdk:10.0-noble AS build
ENV DOTNET_CLI_TELEMETRY_OPTOUT=1 \
    DOTNET_NOLOGO=1 \
    PATH="/usr/local/go/bin:/root/go/bin:${PATH}"

COPY --from=go-toolchain /usr/local/go /usr/local/go
COPY --from=node-toolchain /usr/local/bin/node /usr/local/bin/node
COPY --from=node-toolchain /usr/local/lib/node_modules /usr/local/lib/node_modules
RUN ln -s ../lib/node_modules/npm/bin/npm-cli.js /usr/local/bin/npm \
    && ln -s ../lib/node_modules/npm/bin/npx-cli.js /usr/local/bin/npx \
    && apt-get update \
    && apt-get install --no-install-recommends -y \
        build-essential pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev \
        gcc-mingw-w64-x86-64 g++-mingw-w64-x86-64 \
        git ca-certificates python3 \
    && rm -rf /var/lib/apt/lists/*
RUN go install github.com/wailsapp/wails/v2/cmd/wails@v2.17.0

WORKDIR /src
COPY . .
ARG APP_VERSION
ARG SOURCE_COMMIT
RUN make release APP_VERSION="$APP_VERSION" SOURCE_COMMIT="$SOURCE_COMMIT"

# The local exporter copies only these files back, with no bind-mounted source.
FROM scratch AS release
COPY --from=build /src/build/release-assets/ /
