# One Dockerfile for every Go service. Pick the service with --build-arg SERVICE:
#
#   docker build -f services/Dockerfile --build-arg SERVICE=booking -t opspilot/booking:dev services

# ---- build -------------------------------------------------------------------
FROM golang:1.26.9-bookworm AS build
WORKDIR /src

# Download modules in their own layer so code changes don't refetch them.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

ARG SERVICE
ARG VERSION=dev
ARG COMMIT=dev
RUN test -n "${SERVICE}" && test -d "./cmd/${SERVICE}" \
    || (echo "build arg SERVICE must name a directory under cmd/ (got '${SERVICE}')" >&2 && exit 1)

# Static binary (CGO off), no local paths (-trimpath), no debug info (-s -w),
# version metadata injected with -X.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath \
      -ldflags "-s -w \
        -X github.com/git4jude/opspilot/services/internal/platform/version.Version=${VERSION} \
        -X github.com/git4jude/opspilot/services/internal/platform/version.Commit=${COMMIT}" \
      -o /out/app ./cmd/${SERVICE}

# ---- runtime -----------------------------------------------------------------
# distroless/static: no shell, no package manager, CA certs + tzdata only.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/app /app
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/app"]
