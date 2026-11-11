# Development image for Radar: Go, Node with pnpm, code generators, linters,
# Playwright system dependencies and local utilities (section 8.4).
FROM node:24-bookworm-slim AS node

FROM golang:1.26-bookworm

ARG USERNAME=dev
ARG USER_UID=1000
ARG USER_GID=1000

ARG SQLC_VERSION=v1.31.1
ARG GOOSE_VERSION=v3.30.0
ARG OAPI_CODEGEN_VERSION=v2.8.2
ARG GOLANGCI_LINT_VERSION=v2.15.0
ARG MKCERT_VERSION=v1.4.4

ENV TZ=America/Sao_Paulo \
    GOPATH=/go \
    GOTOOLCHAIN=local \
    PNPM_HOME=/home/dev/.local/share/pnpm \
    PATH=/home/dev/.local/share/pnpm:/go/bin:/usr/local/go/bin:$PATH

# Base utilities used by the Makefile, the icon generator and the backup checks.
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        sudo make git less sqlite3 librsvg2-bin ca-certificates tzdata ffmpeg \
    && rm -rf /var/lib/apt/lists/*

# Node runtime copied from the official image, with corepack providing pnpm.
COPY --from=node /usr/local/bin/node /usr/local/bin/node
COPY --from=node /usr/local/lib/node_modules /usr/local/lib/node_modules
RUN ln -s ../lib/node_modules/npm/bin/npm-cli.js /usr/local/bin/npm \
    && ln -s ../lib/node_modules/npm/bin/npx-cli.js /usr/local/bin/npx \
    && ln -s ../lib/node_modules/corepack/dist/corepack.js /usr/local/bin/corepack \
    && corepack enable

# System libraries needed by Playwright WebKit and Chromium. Browsers are
# downloaded later by `make setup`, matching the version in web/package.json.
RUN npx -y playwright@1.65.0 install-deps webkit chromium \
    && rm -rf /root/.npm /var/lib/apt/lists/*

# Go code generators, linter and mkcert.
RUN go install github.com/sqlc-dev/sqlc/cmd/sqlc@${SQLC_VERSION} \
    && go install github.com/pressly/goose/v3/cmd/goose@${GOOSE_VERSION} \
    && go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@${OAPI_CODEGEN_VERSION} \
    && go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@${GOLANGCI_LINT_VERSION} \
    && go install filippo.io/mkcert@${MKCERT_VERSION} \
    && rm -rf /root/.cache/go-build

# Non root user with the same UID as the WSL user, so files keep the right owner.
RUN groupadd --gid ${USER_GID} ${USERNAME} \
    && useradd --uid ${USER_UID} --gid ${USER_GID} -m -s /bin/bash ${USERNAME} \
    && echo "${USERNAME} ALL=(root) NOPASSWD:ALL" > /etc/sudoers.d/${USERNAME} \
    && chmod 0440 /etc/sudoers.d/${USERNAME} \
    && chown -R ${USERNAME}:${USERNAME} /go \
    && mkdir -p ${PNPM_HOME} && chown -R ${USERNAME}:${USERNAME} /home/${USERNAME}

USER ${USERNAME}
