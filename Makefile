# Radar development commands. Run them inside the Dev Container, except the
# deploy targets, which run in the WSL2 shell where Docker Compose is available.

SHELL := /bin/bash
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

DEV_ENV := RADAR_DB_PATH=tmp/radar.db RADAR_BACKUP_DIR=tmp/backups RADAR_COOKIE_SECURE=false RADAR_ADDR=:8080

.PHONY: setup generate dev dev-api dev-web test test-go test-web e2e lint build icons \
        user-add user-pin group-create certs deploy deploy-logs deploy-user-add deploy-user-pin deploy-group-create deploy-backup

## setup: downloads Go and pnpm dependencies and the Playwright browsers.
setup:
	go mod download
	cd web && pnpm install --frozen-lockfile
	cd web && pnpm exec playwright install webkit chromium

## generate: regenerates sqlc queries, the OpenAPI server and the frontend API types.
generate:
	sqlc generate
	oapi-codegen -config api/oapi-codegen.yaml api/openapi.yaml
	cd web && pnpm run gen:api

## dev: runs the Go API and the Vite dev server together.
dev:
	@$(MAKE) -j2 dev-api dev-web

dev-api:
	@mkdir -p tmp/backups
	$(DEV_ENV) go run ./cmd/radar serve

dev-web:
	cd web && pnpm dev --host 0.0.0.0

## user-add: creates a user in the development database (LOGIN=... NAME=..., optional GROUP=...).
user-add:
	@mkdir -p tmp
	$(DEV_ENV) go run ./cmd/radar user add --login "$(LOGIN)" --name "$(NAME)" $(if $(GROUP),--group "$(GROUP)")

## user-pin: sets the PIN of a development user, typed at the prompt (LOGIN=...).
user-pin:
	$(DEV_ENV) go run ./cmd/radar user pin --login "$(LOGIN)"

## group-create: creates a development group with its first administrator (NAME=... ADMIN=login).
group-create:
	$(DEV_ENV) go run ./cmd/radar group create --name "$(NAME)" --admin "$(ADMIN)"

## test: runs Go tests and the frontend type check.
test: test-go test-web

test-go:
	go test ./...

test-web:
	cd web && pnpm run typecheck

## e2e: runs the Playwright flows, including WebKit with an iPhone viewport.
e2e:
	cd web && pnpm run e2e

## lint: runs golangci-lint and ESLint.
lint:
	golangci-lint run ./...
	cd web && pnpm run lint

## build: builds the frontend and a single Go binary with the frontend embedded.
build:
	cd web && pnpm run build
	CGO_ENABLED=0 go build -tags embedweb -ldflags "$(LDFLAGS)" -o bin/radar ./cmd/radar

## icons: renders the PWA icons from web/icon.svg.
icons:
	rsvg-convert -w 180 -h 180 web/icon.svg -o web/public/apple-touch-icon.png
	rsvg-convert -w 192 -h 192 web/icon.svg -o web/public/icon-192.png
	rsvg-convert -w 512 -h 512 web/icon.svg -o web/public/icon-512.png
	rsvg-convert -w 32 -h 32 web/icon.svg -o web/public/favicon-32.png

## certs: creates the local CA and the HTTPS certificate (HOSTS="radar.local 192.168.0.10").
certs:
	@test -n "$(HOSTS)" || (echo 'Informe HOSTS="nome.local 192.168.x.y"' && exit 1)
	mkdir -p deploy/certs/ca
	CAROOT=$(CURDIR)/deploy/certs/ca mkcert -cert-file deploy/certs/radar.pem -key-file deploy/certs/radar-key.pem $(HOSTS)
	cp deploy/certs/ca/rootCA.pem deploy/certs/radar-root-ca.pem
	@echo "Instale deploy/certs/radar-root-ca.pem em cada iPhone (veja docs/instalacao.md)."

## deploy: builds and (re)starts production with one command (RNF-19). Run in WSL2.
deploy:
	cd deploy && VERSION=$(VERSION) docker compose up -d --build --remove-orphans

deploy-logs:
	cd deploy && docker compose logs -f --tail=100

## deploy-user-add: creates a user in production (LOGIN=... NAME=..., optional GROUP=...). Run in WSL2.
deploy-user-add:
	cd deploy && docker compose exec radar /radar user add --login "$(LOGIN)" --name "$(NAME)" $(if $(GROUP),--group "$(GROUP)")

## deploy-user-pin: sets the PIN of a production user, typed at the prompt (LOGIN=...). Run in WSL2.
deploy-user-pin:
	cd deploy && docker compose exec -i radar /radar user pin --login "$(LOGIN)"

## deploy-group-create: creates a production group with its first administrator (NAME=... ADMIN=login). Run in WSL2.
deploy-group-create:
	cd deploy && docker compose exec radar /radar group create --name "$(NAME)" --admin "$(ADMIN)"

## deploy-backup: runs a manual backup in production. Run in WSL2.
deploy-backup:
	cd deploy && docker compose exec radar /radar backup
