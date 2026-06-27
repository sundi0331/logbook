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
|  checkpoint-enabled  |  LOGBOOK_CHECKPOINT_ENABLED  |  Enable Kubernetes Event resourceVersion checkpointing (default is true)  |
|  checkpoint-backend  |  LOGBOOK_CHECKPOINT_BACKEND  |  Checkpoint backend (default is configmap)<br>Valid values: configmap, file  |
|  checkpoint-namespace  |  LOGBOOK_CHECKPOINT_NAMESPACE  |  Namespace for the checkpoint ConfigMap (default is POD_NAMESPACE or default; Helm sets this to the release namespace)  |
|  checkpoint-name  |  LOGBOOK_CHECKPOINT_NAME  |  Name of the checkpoint ConfigMap (default is logbook-checkpoint)  |
|  checkpoint-path  |  LOGBOOK_CHECKPOINT_PATH  |  Path for the file checkpoint backend (default is logbook-checkpoint)  |
|  checkpoint-flush-interval  |  LOGBOOK_CHECKPOINT_FLUSH_INTERVAL  |  Coalesce checkpoint writes and flush the latest resourceVersion at this interval (default is 5s; set to 0s to flush every event)  |
|  checkpoint-on-expired-resource-version  |  LOGBOOK_CHECKPOINT_ON_EXPIRED_RESOURCE_VERSION  |  Behavior when Kubernetes has compacted the saved resourceVersion (default is skip-existing)<br>Valid values: skip-existing, fail  |
|  leader-election-enabled  |  LOGBOOK_LEADER_ELECTION_ENABLED  |  Enable Kubernetes Lease leader election (default is false for the binary; Helm enables it by default)  |
|  leader-election-namespace  |  LOGBOOK_LEADER_ELECTION_NAMESPACE  |  Namespace for the leader election Lease (default is POD_NAMESPACE or default; Helm sets this to the release namespace)  |
|  leader-election-name  |  LOGBOOK_LEADER_ELECTION_NAME  |  Name of the leader election Lease (default is logbook-leader)  |
|  leader-election-identity  |  LOGBOOK_LEADER_ELECTION_IDENTITY  |  Identity for this leader election candidate (default is POD_NAME or hostname; Helm uses the pod name)  |
|  leader-election-lease-duration  |  LOGBOOK_LEADER_ELECTION_LEASE_DURATION  |  Leader election lease duration (default is 15s)  |
|  leader-election-renew-deadline  |  LOGBOOK_LEADER_ELECTION_RENEW_DEADLINE  |  Leader election renew deadline (default is 10s)  |
|  leader-election-retry-period  |  LOGBOOK_LEADER_ELECTION_RETRY_PERIOD  |  Leader election retry period (default is 2s)  |


## Development

```sh
make verify
make build
```

Logbook watches Kubernetes Events through the stable `events.k8s.io/v1` API.

By default, Logbook persists the last processed Kubernetes Event `resourceVersion` and resumes from it after a restart. Helm deployments use a ConfigMap checkpoint in the release namespace. Checkpoint writes are buffered and flushed every 5 seconds to reduce Kubernetes API and etcd write load. Set `--checkpoint-enabled=false` or `LOGBOOK_CHECKPOINT_ENABLED=false` to opt out.

The `file` checkpoint backend requires a writable checkpoint path. Helm mounts a writable `emptyDir` at `/var/lib/logbook` when `logbook.checkpoint.backend=file`; this survives container restarts in the same Pod, but not Pod replacement or rescheduling. Helm requires `replicaCount=1` for the file backend because each Pod has its own `emptyDir`; use the default `configmap` backend for multiple replicas.

Helm deployments enable Kubernetes Lease leader election by default, so only the elected Logbook replica streams events and writes checkpoints. This prevents duplicate logs when `replicaCount` is greater than 1.

When enabling ConfigMap checkpointing outside the Helm chart, grant `get`, `create`, and `patch` on the checkpoint ConfigMap. When enabling leader election outside the Helm chart, grant `get`, `create`, `update`, and `patch` on the leader election Lease.

Kubernetes Events may contain operationally sensitive object names, namespaces, references, and notes. Treat Logbook output as cluster operational data and route it only to logging systems with appropriate access controls.

## Release Procedure

Releases are tag driven. Create and push a semantic version tag from the commit that should be released:

```sh
git tag -a v1.2.3 -m "v1.2.3"
git push origin v1.2.3
```

The Release workflow validates the `vMAJOR.MINOR.PATCH` tag, builds packaged binaries for Linux, macOS, and Windows, publishes a GitHub Release with `SHA256SUMS`, publishes GHCR release images, scans the image with Trivy, signs the published image tags with cosign, and attaches a cosign-verifiable SPDX SBOM attestation. Manual workflow dispatch is for rerunning an existing tag release; it does not create new tags.
