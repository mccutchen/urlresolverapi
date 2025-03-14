# syntax = docker/dockerfile:1.3
FROM golang:1.24

WORKDIR /go/src/app

COPY go.mod go.sum ./
RUN --mount=type=cache,id=gomod,target=/go/pkg/mod \
    go mod download

COPY . .
RUN --mount=type=cache,id=gomod,target=/go/pkg/mod \
    --mount=type=cache,id=gobuild,target=/root/.cache/go-build \
    BUILD_ARGS='-mod=readonly -ldflags="-s -w" -buildvcs=false' \
    DIST_PATH=/bin \
    make

FROM gcr.io/distroless/base
COPY --from=0 /bin/urlresolverapi /bin/urlresolverapi
CMD ["/bin/urlresolverapi"]
