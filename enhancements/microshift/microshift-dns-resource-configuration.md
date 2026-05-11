---
title: microshift-dns-resource-configuration
authors:
  - "@Neilhamza"
reviewers:
  - "@pacevedom"
  - "@eslutsky"
  - "@copejon"
  - "@ggiguash"
  - "@pmtk"
  - "@dfroehli, for customer requirements and use case validation"
approvers:
  - "@jerpeter1"
api-approvers:
  - None
creation-date: 2026-04-28
last-updated: 2026-04-28
tracking-link:
  - https://redhat.atlassian.net/browse/OCPSTRAT-3015
see-also:
  - enhancements/microshift/microshift-coredns-hosts.md
---

# MicroShift DNS Deployment Resource Configuration

## Summary

MicroShift deploys CoreDNS as a DaemonSet (`dns-default`) in the `openshift-dns` namespace with hardcoded resource requests (cpu: 50m, memory: 70Mi) and no resource limits. This enhancement introduces a new `dns.resources` configuration option that allows administrators to override the default CPU and memory resources (requests and limits) for the `dns` container in the `dns-default` DaemonSet.

## Motivation

Edge deployments have diverse resource profiles. Devices running high DNS query volumes or custom CoreDNS plugins may need more CPU and memory than the defaults.
Conversely, extremely constrained devices may benefit from reducing the resource requests to free capacity for application workloads.
Currently, the only way to change DNS pod resources is to manually patch the DaemonSet after MicroShift starts, which is overwritten on the next restart.
Customers need a supported, persistent way to tune DNS pod resources through the MicroShift configuration file.

### User Stories

1. As a MicroShift administrator running high DNS query workloads, I want to increase the CPU and memory allocated to CoreDNS so that DNS resolution remains responsive under load.
2. As a MicroShift administrator on a resource-constrained edge device, I want to adjust the DNS pod resource allocation to balance DNS reliability with available capacity for application workloads.
3. As a MicroShift administrator, I want to set resource limits on the DNS pod to prevent it from consuming excessive resources on shared nodes.

### Goals

1. Allow users to configure CPU and memory resource requests for the `dns` container in the `dns-default` DaemonSet via the MicroShift configuration file.
2. Allow users to configure CPU and memory resource limits for the `dns` container.
3. Preserve the current default resource values when the option is not set, ensuring full backward compatibility.
4. Validate user-provided resource values at startup and fail with a clear error message if they are invalid.

### Non-Goals

1. Configuring resources for the `kube-rbac-proxy` sidecar container in the `dns-default` DaemonSet.
2. Configuring resources for the `node-resolver` DaemonSet.
3. Runtime resource updates without restarting MicroShift - a restart is required for resource changes to take effect.
4. Allowing resource requests below the shipped defaults (cpu: 50m, memory: 70Mi).

## Proposal

Introduce a new optional `dns.resources` section in the MicroShift configuration file. When set, the values are used directly as the Kubernetes `resources` spec for the `dns` container in the `dns-default` DaemonSet. When not set, the current hardcoded defaults are preserved.

**Example configuration:**

```yaml
dns:
  resources:
    requests:
      cpu: 150m
      memory: 120Mi
    limits:
      cpu: 150m
      memory: 120Mi
```

The `resources` section maps 1:1 into the `dns` container's resource specification. Users can provide `requests` only, `limits` only, or both.

### Workflow Description

1. MicroShift starts up and loads its configuration file.
2. If `dns.resources` is set, the values are validated (each value must be a valid Kubernetes resource quantity).
3. If validation fails, MicroShift exits with a descriptive error message.
4. If validation succeeds, the resource values are passed as template parameters when rendering the `dns-default` DaemonSet.
5. The rendered DaemonSet is applied to the cluster with the user-specified resources for the `dns` container.
6. If `dns.resources` is not set, the default values (cpu: 50m, memory: 70Mi requests, no limits) are used.

### API Extensions

The following changes to the MicroShift configuration file are proposed:

```yaml
dns:
  resources:
    requests:
      cpu: <resource.Quantity>    # optional, default: 50m
      memory: <resource.Quantity> # optional, default: 70Mi
    limits:
      cpu: <resource.Quantity>    # optional, default: not set
      memory: <resource.Quantity> # optional, default: not set
```

By default, the `dns.resources` section is not set, and the current hardcoded defaults are used. When provided, individual fields within `requests` and `limits` are optional - users can set only the values they want to override. Unset request fields preserve their defaults via key-by-key merge (e.g., setting only `requests.cpu` preserves the default `memory: 70Mi`). Only `cpu` and `memory` keys are accepted; unsupported keys are rejected during validation.

When limits are set without overriding requests, the default requests (cpu: 50m, memory: 70Mi) are preserved. Note that if a limit is set lower than the corresponding default request (e.g., `limits.cpu: 30m` without overriding `requests.cpu: 50m`), MicroShift will fail to start because validation requires limits >= requests.

### Topology Considerations

#### Hypershift / Hosted Control Planes
N/A

#### Standalone Clusters
N/A

#### Single-node Deployments or MicroShift
Enhancement is intended for MicroShift only.

#### OpenShift Kubernetes Engine
N/A

### Implementation Details/Notes/Constraints

#### Config struct changes

Add a `Resources` field to the `DNS` struct in `pkg/config/dns.go`:

```go
type DNS struct {
    BaseDomain string       `json:"baseDomain"`
    Hosts      HostsConfig  `json:"hosts,omitempty"`
    Resources  DNSResources `json:"resources,omitempty"`
}

type DNSResources struct {
    Requests map[string]string `json:"requests,omitempty"`
    Limits   map[string]string `json:"limits,omitempty"`
}
```

A value type (not pointer) is used, consistent with other config fields like `HostsConfig`. When not set by the user, the nil maps are detected in `incorporateUserSettings()` and defaults from `dnsDefaults()` are preserved.

#### Config validation

Extend the `DNS.validate()` method in `pkg/config/dns.go`:

- Only `cpu` and `memory` keys are accepted in `Requests` and `Limits`. Unsupported keys (e.g., `gpu`) are rejected with a clear error.
- Each value must be parseable by `resource.ParseQuantity()`.
- Resource requests must meet minimum thresholds: cpu >= 50m, memory >= 70Mi. These match the shipped defaults and prevent unstable clusters from insufficient DNS resources.
- If a resource has both a request and a limit, the limit must be greater than or equal to the request.

#### Config incorporation

Add incorporation logic in `pkg/config/config.go` `incorporateUserSettings()` using key-by-key merge to preserve defaults for unset fields:

```go
if u.DNS.Resources.Requests != nil {
    if c.DNS.Resources.Requests == nil {
        c.DNS.Resources.Requests = make(map[string]string)
    }
    for k, v := range u.DNS.Resources.Requests {
        c.DNS.Resources.Requests[k] = v
    }
}
if u.DNS.Resources.Limits != nil {
    if c.DNS.Resources.Limits == nil {
        c.DNS.Resources.Limits = make(map[string]string)
    }
    for k, v := range u.DNS.Resources.Limits {
        c.DNS.Resources.Limits[k] = v
    }
}
```

This ensures that setting only `requests.cpu` preserves the default `requests.memory`.

#### Template rendering

Modify `assets/components/openshift-dns/dns/daemonset.yaml` to use template variables for the `dns` container resources instead of hardcoded values:

```yaml
resources:
  requests:
    cpu: {{ .DNSCPURequest }}
    memory: {{ .DNSMemoryRequest }}
  {{- if .DNSHasLimits }}
  limits:
    {{- if .DNSCPULimit }}
    cpu: {{ .DNSCPULimit }}
    {{- end }}
    {{- if .DNSMemoryLimit }}
    memory: {{ .DNSMemoryLimit }}
    {{- end }}
  {{- end }}
```

The `DNSHasLimits` boolean controls whether the limits block is rendered at all. Individual limit values are also conditionally rendered so users can set only cpu or only memory limits.

#### Controller changes

Extend `startDNSController()` in `pkg/components/controllers.go` to pass resource values as extra render parameters. Defaults are always populated by `dnsDefaults()`, so the map lookups always return valid values for requests. Nil map access for limits safely returns empty strings, which are falsy in Go templates:

```go
extraParams := assets.RenderParams{
    "ClusterIP":        cfg.Network.DNS,
    "HostsEnabled":     cfg.DNS.Hosts.Status == config.HostsStatusEnabled,
    "DNSCPURequest":    cfg.DNS.Resources.Requests["cpu"],
    "DNSMemoryRequest": cfg.DNS.Resources.Requests["memory"],
    "DNSCPULimit":      cfg.DNS.Resources.Limits["cpu"],
    "DNSMemoryLimit":   cfg.DNS.Resources.Limits["memory"],
    "DNSHasLimits":     len(cfg.DNS.Resources.Limits) > 0,
}
```

### Risks and Mitigations

**Risk:** Users configure resources too low, causing CoreDNS to be OOM-killed or throttled, making the cluster unable to reach readiness.
**Mitigation:** Validation enforces minimum resource requests matching the shipped defaults (cpu: 50m, memory: 70Mi). Requests below these thresholds are rejected at startup. Users can increase resources above the minimums but cannot lower them below proven-stable values.

**Risk:** Users configure resource limits without overriding requests.
**Mitigation:** Default requests (cpu: 50m, memory: 70Mi) are always populated by `dnsDefaults()` and preserved via key-by-key merge. The rendered DaemonSet always contains explicit requests, so Kubernetes never needs to apply its default "request equals limit" behavior. Validation ensures limits >= requests, so setting a limit lower than the default request (e.g., `limits.cpu: 30m` without overriding `requests.cpu: 50m`) is rejected at startup with a clear error.

### Drawbacks
N/A

## Test Plan

### Unit Tests

- Valid configuration: requests only, limits only, both requests and limits.
- Invalid configuration: non-parseable quantities (e.g., "abc").
- Unsupported resource keys (e.g., "gpu") rejected.
- Validation: limits less than requests.
- Limit without corresponding request passes validation.
- Default behavior: no `dns.resources` set, defaults preserved.
- Partial requests: setting only cpu preserves default memory.

### Integration Tests (Robot Framework)

- Default resources: verify `dns-default` DaemonSet uses defaults (cpu: 50m, memory: 70Mi) when not configured.
- Custom resources with requests and limits: configure via drop-in, restart, verify DaemonSet has configured values.
- Requests only: configure only requests, verify no limits are injected.
- Partial requests: configure only cpu request, verify memory default is preserved.
- Limits only: configure only limits without requests, verify default requests (cpu: 50m, memory: 70Mi) are preserved alongside the configured limits.
- Invalid resource quantity: set invalid value, verify MicroShift fails to start with clear error in journal.
- Limit less than request: set limit below request, verify MicroShift fails to start.
- DNS resolution after resource change: apply custom resources, verify CoreDNS still resolves cluster-local services.

## Graduation Criteria

### Dev Preview -> Tech Preview
N/A

### Tech Preview -> GA

- Ability to utilize the enhancement end to end
- End user documentation completed and published
- Available by default
- End-to-end tests
- Unit tests covering config validation

### Removing a deprecated feature
N/A

## Upgrade / Downgrade Strategy

When upgrading from a version without `dns.resources` support, the new configuration field will remain unset, causing the existing defaults to be used. No user action is required on upgrade.

When downgrading to a version without `dns.resources` support, the field is ignored by the older version. MicroShift reverts to the hardcoded default resources on startup. The configuration file on disk is not modified.

## Version Skew Strategy
N/A

## Operational Aspects of API Extensions

When resource limits are configured, the DNS container is subject to standard Kubernetes enforcement. If the container exceeds its memory limit, the kubelet will OOM-kill it. If it exceeds its CPU limit, it will be throttled. In both cases, the DaemonSet controller will automatically restart the container. Administrators should monitor DNS pod restarts (`oc get pods -n openshift-dns`) after changing resource limits to ensure the configured values are sufficient for their workload.

## Support Procedures
N/A

## Alternatives (Not Implemented)

An alternative approach would be to allow users to provide a complete DaemonSet overlay or patch. This was rejected because:
- It exposes too much of the DaemonSet spec surface area, increasing the risk of misconfiguration.
- A focused `dns.resources` option is simpler, safer, and consistent with MicroShift's philosophy of opinionated defaults with targeted overrides.
