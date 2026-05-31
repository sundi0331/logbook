# syntax=docker/dockerfile:1

ARG GO_VERSION=1.26.3

FROM golang:${GO_VERSION} AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . ./

ARG VERSION=dev
ARG COMMIT=unknown
ARG DATE=unknown
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build \
      -trimpath \
      -ldflags="-s -w -X github.com/sundi0331/logbook/cmd.version=${VERSION} -X github.com/sundi0331/logbook/cmd.commit=${COMMIT} -X github.com/sundi0331/logbook/cmd.date=${DATE}" \
      -o /out/logbook .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/logbook /logbook
USER nonroot:nonroot
ENTRYPOINT ["/logbook"]
