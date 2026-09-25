# Go is not installed on the host; everything runs in a container.
# CACHE is a host-side, user-owned directory so modules and build artifacts
# survive between invocations. It must be created by us, not by Docker --
# Docker would create a missing bind-mount source as root.
CACHE = $(HOME)/.cache/mass-mailer

GORUN = docker run --rm -u $(shell id -u):$(shell id -g) \
	-v $(PWD):/src -w /src -v $(CACHE):/tmp/gocache \
	-e GOCACHE=/tmp/gocache/build -e GOMODCACHE=/tmp/gocache/mod \
	-e HOME=/tmp golang:1.26

.PHONY: build test tidy vet cache dirs up up-dev down fix-perms

cache:
	@mkdir -p $(CACHE)

# Create bind-mount sources before compose runs so Docker does not create them
# implicitly.
dirs:
	@mkdir -p data attachments

up: dirs
	docker compose up --build -d

# App plus the Mailpit SMTP sink: web UI on :8080, captured mail on :8025.
up-dev: dirs
	docker compose --profile dev up --build -d

down:
	docker compose --profile dev down

# Repair a data/ directory that Docker already created as root. Runs chown from
# a root container so no host sudo is needed.
fix-perms:
	@mkdir -p data attachments
	docker run --rm -v $(PWD)/data:/data -v $(PWD)/attachments:/attachments \
		busybox chown -R $(shell id -u):$(shell id -g) /data /attachments
	@ls -ldn data attachments

build: cache
	$(GORUN) go build ./...

test: cache
	$(GORUN) go test ./...

tidy: cache
	$(GORUN) go mod tidy

vet: cache
	$(GORUN) go vet ./...
