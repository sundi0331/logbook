# Logbook
[![.github/workflows/build.yml](https://github.com/sundi0331/logbook/actions/workflows/build.yml/badge.svg?branch=main&event=push)](https://github.com/sundi0331/logbook/actions/workflows/build.yml)
[![CodeQL](https://github.com/sundi0331/logbook/actions/workflows/codeql-analysis.yml/badge.svg?branch=main&event=push)](https://github.com/sundi0331/logbook/actions/workflows/codeql-analysis.yml)

Logbook is a kubernetes event logger which can be used both in-cluster(use kubernetes ServiceAccount for auth) and out-of-cluster(use kubeconfig file for auth). It logs kubernetes events to stdout or a file, which can be further processed by your logging pipeline, enabling you manage kubernetes events like container logs.

![logbook helm demo](img/helm-demo.gif)

## Installation
---
### Binary
Download a binary from [**Releases page**](https://github.com/sundi0331/logbook/releases)
```sh
# By default, it will use $HOME/.kube/config to authenticate, you can specify a kubeconfig file using --kubeconfig flag
./logbook --mode=out-of-cluster
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
|  log-filename  |  LOGBOOK_LOG_FILENAME  |  Full path of log file with filename (valid only when log-out is set to file. Default is k8s-events.log in the same directory as logbook)  |
