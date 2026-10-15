SERVICES := gateway events booking payment ticketing notify migrate

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo dev)
LDFLAGS := -s -w \
	-X github.com/git4jude/opspilot/services/internal/platform/version.Version=$(VERSION) \
	-X github.com/git4jude/opspilot/services/internal/platform/version.Commit=$(COMMIT)

COMPOSE := docker compose -f ../compose.yaml

.PHONY: build test vet docker up down $(addprefix docker-,$(SERVICES))

## build: compile every service into ./bin
build:
	@mkdir -p bin
	@for s in $(SERVICES); do \
		echo "building $$s"; \
		CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/$$s ./cmd/$$s || exit 1; \
	done

## test: run unit tests
test:
	go test ./...

## vet: run go vet
vet:
	go vet ./...

## docker: build every image as opspilot/<service>:dev
docker: $(addprefix docker-,$(SERVICES))

docker-%:
	docker build -f Dockerfile \
		--build-arg SERVICE=$* --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) \
		-t opspilot/$*:dev .

## up: build and start the whole app with Docker Compose
up:
	$(COMPOSE) up -d --build

## down: stop the app (add ARGS=-v to also delete volumes)
down:
	$(COMPOSE) down $(ARGS)
