GO ?= go
BINARY ?= logbook
VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w -X github.com/sundi0331/logbook/cmd.version=$(VERSION) -X github.com/sundi0331/logbook/cmd.commit=$(COMMIT) -X github.com/sundi0331/logbook/cmd.date=$(DATE)

.PHONY: all fmt vet test tidy build build-linux-amd64 build-win-amd64 helm-lint helm-template helm-template-long-names docker-build smoke-kind verify clean

all: verify build

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

test:
	$(GO) test ./...

tidy:
	$(GO) mod tidy

build:
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(BINARY) .

build-linux-amd64:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(BINARY)_linux_amd64 .

build-win-amd64:
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(BINARY)_windows_amd64.exe .

helm-lint:
	command -v helm >/dev/null
	helm lint ./helmchart

helm-template:
	command -v helm >/dev/null
	helm template logbook ./helmchart

helm-template-long-names:
	command -v helm >/dev/null
	helm template aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa ./helmchart | awk '/^kind: /{kind=$$2} /^metadata:/{inmeta=1} inmeta && /^  name: /{name=$$2; if (length(name) > 63) {printf "%s metadata.name %s has length %d\n", kind, name, length(name); invalid=1}} /^(spec|rules|roleRef|subjects|data):/{inmeta=0} END{exit invalid}'

docker-build:
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg DATE=$(DATE) \
		-t logbook:$(VERSION) .

smoke-kind:
	CREATE_CLUSTER=true bash hack/kind-smoke-test.sh

verify: fmt vet test helm-lint helm-template helm-template-long-names

clean:
	rm -f $(BINARY) $(BINARY)_linux_amd64 $(BINARY)_windows_amd64.exe
