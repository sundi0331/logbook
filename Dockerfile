# Build stage
FROM golang:1.23 as build

COPY ./ /go/src/logbook
WORKDIR /go/src/logbook
RUN curl -sfL https://raw.githubusercontent.com/aquasecurity/trivy/main/contrib/install.sh | sh -s -- -b /usr/local/bin && \
    trivy config ./ && \
    trivy fs . && \
    go mod tidy && \
    go build -o /go/bin/logbook

# Output stage
FROM gcr.io/distroless/base
USER 100000
COPY --from=build /go/bin/logbook /
CMD ["/logbook"]
