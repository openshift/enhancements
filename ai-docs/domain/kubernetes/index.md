# Kubernetes Domain Concepts

Core Kubernetes APIs fundamental to OpenShift.

## APIs

- [crds.md](crds.md) - Custom Resource Definitions (extending Kubernetes API)
- [pod.md](pod.md) - Smallest deployable unit (containers, probes, resources)
- [service.md](service.md) - Stable networking and service discovery

## Related

- **Full API reference**: `oc api-resources` or [Kubernetes API Docs](https://kubernetes.io/docs/reference/kubernetes-api/)
- **Field details**: `oc explain <resource>` (e.g., `oc explain pod.spec`)
- **OpenShift APIs**: [../openshift/](../openshift/)
