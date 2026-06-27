# AGENTS.md

## Project overview

Logbook is a Go-based Kubernetes event logger. It runs either in-cluster with a Kubernetes ServiceAccount or out-of-cluster with a kubeconfig.

It watches Kubernetes Events through the stable `events.k8s.io/v1` API and writes observed watch events to the configured log sink. In Helm deployments, checkpointing and leader election are enabled by default so restarts avoid replaying already processed Events and multiple replicas do not duplicate the stream.

## Development workflow

- Follow TDD for behavior changes:
  - Write or update a failing test that describes the desired behavior before changing implementation code.
  - Make the smallest implementation change that makes the test pass.
  - Refactor only after tests are green.
- Keep tests focused on the affected behavior. Broaden coverage when changing shared runtime paths such as event watching, checkpointing, leader election, config loading, logging, or Helm chart output.
- For bug fixes, add a regression test that would fail without the fix.
- For config or Helm changes, update the docs, sample config, `values.yaml`, `values.schema.json`, templates, and tests/validation together.
- Do not silently change defaults. If a default changes, update README and Helm values comments in the same change.

## Go conventions

- Target the Go version declared in `go.mod`.
- Prefer standard-library features when practical.
- Use `log/slog` for structured logging.
- Keep accepted log levels to `debug`, `info`, `warn`, and `error`.
- Return errors instead of panicking from application or library code.
- Pass `context.Context` through long-running operations.
- Avoid package-level mutable state unless it is required by a CLI framework.
- Run `gofmt` on all Go files.
- Keep long-running loops cancellable through `context.Context`.
- Avoid unnecessary Kubernetes API writes. Coalesce or debounce writes where practical.
- When adding a config field, expose it consistently through struct config, Cobra flags, Viper/env binding, sample YAML, README, and Helm when applicable.
- When adding dependencies, run `go mod tidy` and inspect `go.mod`/`go.sum` changes.

## Kubernetes conventions

- Use the stable `events.k8s.io/v1` Events API.
- Keep RBAC least-privilege.
- Follow restricted Pod Security Standards where practical.
- Quote Helm-rendered environment variable values.
- Use `coordination.k8s.io/v1` Leases for leader election.
- Helm deployments should keep leader election enabled by default.
- Store ConfigMap checkpoints through the Kubernetes API, not a mounted ConfigMap volume.
- Keep checkpoint writes buffered by default to avoid excessive API server and etcd write load.
- When changing Kubernetes API calls, verify the Helm RBAC verbs match the actual client-go methods used.
- Keep Helm `values.schema.json` in sync with supported values.
- Kubernetes Events can contain sensitive operational information. Preserve README/security notes when changing event output behavior.

## Docker conventions

- Use multi-stage builds.
- Do not run vulnerability scanners inside the Dockerfile; run them in CI instead.
- Prefer non-root, minimal runtime images.
- Keep `.dockerignore` up to date.

## GitHub Actions conventions

- Use least-privilege `permissions` at workflow and job scope.
- Keep CI, container publishing, release, and security scanning responsibilities clear.
- Avoid mutable third-party refs such as `@main` and `@master`.
- Prefer `go-version-file: go.mod` for Go setup.

## Validation

Run the relevant subset before submitting changes:

- `go test ./...`
- `go vet ./...`
- `go mod tidy`
- `helm lint ./helmchart`
- `helm template logbook ./helmchart`
- `docker build .`

If the default Go build or module cache is not writable in the development environment, use temporary caches, for example:

- `GOCACHE=/tmp/logbook-go-cache GOMODCACHE=/tmp/logbook-go-mod-cache go test ./...`
- `GOCACHE=/tmp/logbook-go-cache GOMODCACHE=/tmp/logbook-go-mod-cache go vet ./...`
