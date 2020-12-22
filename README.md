# nkml

Node Kernel Module Labeler - label kubernetes nodes, according to their kernel modules.

## Usage

Apply the example configuration
```bash
kubectl apply -f https://raw.githubusercontent.com/leonnicolas/nkml/main/example.yaml
```

### Configure the Labeler
```bash
Usage of ./nkml:
      --hostname string         Hostname of the node on which this process is running
      --kubeconfig string       path to kubeconfig
  -m, --label-mod strings       list of strings, kernel modules matching a string will be used as labels with values true, if found
      --label-prefix string     prefix for labels (default "nudl.squat.ai")
      --listen-address string   listen address for prometheus metrics server (default ":8080")
      --log-level string        Log level to use. Possible values: all, debug, info, warn, error, none (default "info")
      --update-time duration    renewal time for labels in seconds (default 10s)
```

__Note:__ to check kernel modules, __nudl__ needs access to the file _/proc/modules_.

### Label Kernel Modules
You can label your nodes according to its kernel modules. Use the --label-mod flag to pass a list of strings. If a kernel module found in __/proc/modules__ matches one of the input strings a label of the format:
```
<label prefix>/<module name>=true
```
for exampel:
```
nudle.example.com/wireguard=true
```
If the module is not found, the flag's value will be set to _fasle_.
 
### Outside the cluster

```bash
docker run --rm -v ~/.kube:/mnt leonnicolas/nkml --kubeconfig /mnt/k3s.yaml --label-mod="wireguard,fantasy" --hostname example_host
```
