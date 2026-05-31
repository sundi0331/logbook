# AGENTS.md

## Project overview

Logbook is a Go-based Kubernetes event logger. It runs either in-cluster with a Kubernetes ServiceAccount or out-of-cluster with a kubeconfig.

## Go conventions

- Target the Go version declared in `go.mod`.
- Prefer standard-library features when practical.
- Use `log/slog` for structured logging.
- Keep accepted log levels to `debug`, `info`, `warn`, and `error`.
- Return errors instead of panicking from application or library code.
- Pass `context.Context` through long-running operations.
- Avoid package-level mutable state unless it is required by a CLI framework.
- Run `gofmt` on all Go files.

## Kubernetes conventions

- Use the stable `events.k8s.io/v1` Events API.
- Keep RBAC least-privilege.
- Follow restricted Pod Security Standards where practical.
- Quote Helm-rendered environment variable values.

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
