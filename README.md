# nkml

Node Kernel Module Labeler - label Kubernetes nodes according to their kernel modules.

[![ci](https://github.com/leonnicolas/nkml/actions/workflows/push.yaml/badge.svg)](https://github.com/leonnicolas/nkml/actions/workflows/push.yaml)
[![Docker Pulls](https://img.shields.io/docker/pulls/leonnicolas/nkml?color=blue)](https://hub.docker.com/r/leonnicolas/nkml)

## Usage

Apply the example configuration
```bash
kubectl apply -f https://raw.githubusercontent.com/leonnicolas/nkml/main/example.yaml
```
to label nodes with regard to the [WireGuard](https://www.wireguard.com/) kernel module.

### Configure the Labeler
```
Usage of ./nkml:
      --hostname string         Hostname of the node on which this process is running
      --kubeconfig string       path to kubeconfig
  -m, --label-mod strings       list of strings, kernel modules matching a string will be used as labels with values true, if found
      --label-prefix string     prefix for labels (default "nkml.squat.ai")
      --listen-address string   listen address for prometheus metrics server (default ":8080")
      --log-level string        Log level to use. Possible values: all, debug, info, warn, error, none (default "info")
      --no-clean-up             Will not attempt to clean labels before shutting down
      --update-time duration    renewal time for labels in seconds (default 10s)
```

__Note:__ to check kernel modules, __nkml__ needs access to the file _/proc/modules_.

### Label Kernel Modules
You can label your nodes according to their kernel modules.
Use the `--label-mod` flag to pass a list of strings.
If a kernel module found in __/proc/modules__ matches one of the input strings, then the node will be given a label of the format:
```	
<label prefix>/<module name>=true	
```	
for example:	
```	
nkml.squat.ai/wireguard=true	
```	
If the module is not found, the flag's value will be set to _false_.
 
### Outside the cluster

```bash
docker run --rm -v ~/.kube:/mnt leonnicolas/nkml --kubeconfig /mnt/k3s.yaml --label-mod="wireguard,fantasy" --hostname example_host
```

## Images

Images can be found on [Docker Hub](https://hub.docker.com/r/leonnicolas/nkml) `leonnicolas/nkml` and [GitHub Container Registry](https://ghcr.io) `ghcr.io/leonnicolas/nkml`.
