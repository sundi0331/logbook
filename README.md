# Logbook
[![CI](https://github.com/sundi0331/logbook/actions/workflows/ci.yml/badge.svg?branch=dev&event=push)](https://github.com/sundi0331/logbook/actions/workflows/ci.yml)
[![CodeQL](https://github.com/sundi0331/logbook/actions/workflows/codeql.yml/badge.svg?branch=dev&event=push)](https://github.com/sundi0331/logbook/actions/workflows/codeql.yml)
[![Container Image](https://github.com/sundi0331/logbook/actions/workflows/container-image.yml/badge.svg?branch=dev&event=push)](https://github.com/sundi0331/logbook/actions/workflows/container-image.yml)

Logbook is a Kubernetes event logger which can be used either in-cluster(use kubernetes ServiceAccount for auth) or out-of-cluster(use kubeconfig file for auth). It logs kubernetes events to stdout or a file, which can be further processed by your logging pipeline, enabling you manage kubernetes events like container logs.

![logbook helm demo](img/helm-demo.gif)

## Installation
---
### Binary
Download a binary archive from [**Releases page**](https://github.com/sundi0331/logbook/releases) and verify it with the release `SHA256SUMS` file.
```sh
# By default, it will use $HOME/.kube/config to authenticate, you can specify a kubeconfig file using --kubeconfig flag
./logbook --mode=out-of-cluster
```

### Container
Release images are published to GitHub Container Registry.
```sh
docker pull ghcr.io/sundi0331/logbook:v1.2.3
docker pull ghcr.io/sundi0331/logbook:1.2.3
```

### Helm
```sh
git clone https://github.com/sundi0331/logbook.git
cd logbook
helm install RELEASE_NAME ./helmchart --namespace=INSTALL_NAMESPACE
```

## Command Flags & Environment Variables
---
|  Flag  |  ENV  |  Description |
| ---- | ---- | ---- |
|  mode  |  LOGBOOK_AUTH_MODE  |  Running mode (default is in-cluster mode)<br>Valid values: in-cluster, out-of-cluster  |
|  kubeconfig  |  LOGBOOK_AUTH_KUBECONFIG  |  Absolute path of kubeconfig file (default is $HOME/.kube/config, only used in out-of-cluster mode)  |
|  namespace  |  LOGBOOK_TARGET_NAMESPACE  |  Namespace to watch (default is all namespaces)  |
|  log-format  |  LOGBOOK_LOG_FORMAT  |  Log format (default is json)<br>Valid values: json, text  |
|  log-out  |  LOGBOOK_LOG_OUT  |  Log output (default is stdout)<br>Valid values: stdout, stderr, file  |
|  log-level  |  LOGBOOK_LOG_LEVEL  |  Log level (default is info)<br>Valid values: debug, info, warn, error  |
|  log-filename  |  LOGBOOK_LOG_FILENAME  |  Full path of log file with filename (valid only when log-out is set to file. Default is k8s-events.log in the same directory as logbook)  |


## Development

```sh
make verify
make build
```

Logbook watches Kubernetes Events through the stable `events.k8s.io/v1` API.

## Release Procedure

Releases are tag driven. Create and push a semantic version tag from the commit that should be released:

```sh
git tag -a v1.2.3 -m "v1.2.3"
git push origin v1.2.3
```

The Release workflow validates the `vMAJOR.MINOR.PATCH` tag, builds packaged binaries for Linux, macOS, and Windows, publishes a GitHub Release with `SHA256SUMS`, publishes GHCR release images, scans the image with Trivy, signs the published image tags with cosign, and attaches a cosign-verifiable SPDX SBOM attestation. Manual workflow dispatch is for rerunning an existing tag release; it does not create new tags.
