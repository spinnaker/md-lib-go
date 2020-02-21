# syntax = docker/dockerfile:experimental
FROM golang:1.13 AS gobuild
# ENV GO111MODULE on
WORKDIR /go/src/github.com/spinnaker/md-lib-go
RUN --mount=type=bind,target=/go/src/github.com/spinnaker/md-lib-go,ro \
    --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg \
    CGO_ENABLED=0 go build -gcflags="-e"
RUN --mount=type=bind,target=/go/src/github.com/spinnaker/md-lib-go,ro \
    --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg \
    go test -v ./...
RUN --mount=type=cache,target=/root/.cache/go-build \
    GOBIN=/root/go/bin go get -v -u github.com/golangci/golangci-lint/cmd/golangci-lint
RUN --mount=type=bind,target=/go/src/github.com/spinnaker/md-lib-go,ro \
    --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg \
    /root/go/bin/golangci-lint run --fast
