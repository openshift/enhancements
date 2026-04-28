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
2. As a MicroShift administrator on a resource-constrained edge device, I want to reduce the DNS pod resource requests so that more capacity is available for application workloads.
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
4. Validating whether the configured resources are sufficient for CoreDNS to function correctly.

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

By default, the `dns.resources` section is not set, and the current hardcoded defaults are used. When provided, individual fields within `requests` and `limits` are optional - users can set only the values they want to override.

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

Add a `Resources` pointer field to the `DNS` struct in `pkg/config/dns.go`:

```go
type DNS struct {
    BaseDomain string        `json:"baseDomain"`
    Hosts      HostsConfig   `json:"hosts,omitempty"`
    Resources  *DNSResources `json:"resources,omitempty"`
}

type DNSResources struct {
    Requests map[string]string `json:"requests,omitempty"`
    Limits   map[string]string `json:"limits,omitempty"`
}
```

Using a pointer allows distinguishing between "not set" (nil, use defaults) and "set to empty" (explicit zero resources).

#### Config validation

Extend the `DNS.validate()` method in `pkg/config/dns.go`:

- Each value in `Requests` and `Limits` must be parseable by `resource.ParseQuantity()`.
- If a resource has both a request and a limit, the limit must be greater than or equal to the request.

#### Config incorporation

Add incorporation logic in `pkg/config/config.go` `incorporateUserSettings()`:

```go
if u.DNS.Resources != nil {
    c.DNS.Resources = u.DNS.Resources
}
```

#### Template rendering

Modify `assets/components/openshift-dns/dns/daemonset.yaml` to use template variables for the `dns` container resources instead of hardcoded values:

```yaml
resources:
  requests:
    cpu: {{ .DNSCPURequest }}
    memory: {{ .DNSMemoryRequest }}
  {{- if or .DNSCPULimit .DNSMemoryLimit }}
  limits:
    {{- if .DNSCPULimit }}
    cpu: {{ .DNSCPULimit }}
    {{- end }}
    {{- if .DNSMemoryLimit }}
    memory: {{ .DNSMemoryLimit }}
    {{- end }}
  {{- end }}
```

#### Controller changes

Extend `startDNSController()` in `pkg/components/controllers.go` to pass resource values as extra render parameters, with defaults matching the current hardcoded values:

```go
extraParams := assets.RenderParams{
    "ClusterIP":        cfg.Network.DNS,
    "HostsEnabled":     cfg.DNS.Hosts.Status == config.HostsStatusEnabled,
    "DNSCPURequest":    dnsResourceValue(cfg.DNS.Resources, "requests", "cpu", "50m"),
    "DNSMemoryRequest": dnsResourceValue(cfg.DNS.Resources, "requests", "memory", "70Mi"),
    "DNSCPULimit":      dnsResourceValue(cfg.DNS.Resources, "limits", "cpu", ""),
    "DNSMemoryLimit":   dnsResourceValue(cfg.DNS.Resources, "limits", "memory", ""),
}
```

### Risks and Mitigations

**Risk:** Users configure resources too low, causing CoreDNS to be OOM-killed or throttled.
**Mitigation:** Document minimum recommended values. MicroShift does not enforce minimum thresholds to allow flexibility on constrained devices.

**Risk:** Users configure resource limits without requests, leading to unexpected Kubernetes scheduling behavior.
**Mitigation:** When limits are set without requests, Kubernetes defaults requests to equal limits. Document this behavior.

### Drawbacks
N/A

## Test Plan

### Unit Tests

- Valid configuration: requests only, limits only, both requests and limits.
- Invalid configuration: non-parseable quantities, negative values.
- Validation: limits less than requests.
- Default behavior: no `dns.resources` set, defaults preserved.

### Integration Tests (Robot Framework)

- Configure `dns.resources` via drop-in config, restart MicroShift, verify the `dns-default` DaemonSet's `dns` container has the configured resource values.
- Verify default resources are used when `dns.resources` is not configured.
- Verify MicroShift fails to start with an invalid resource quantity and produces a clear error in the journal logs.

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
N/A

## Support Procedures
N/A

## Alternatives (Not Implemented)

An alternative approach would be to allow users to provide a complete DaemonSet overlay or patch. This was rejected because:
- It exposes too much of the DaemonSet spec surface area, increasing the risk of misconfiguration.
- A focused `dns.resources` option is simpler, safer, and consistent with MicroShift's philosophy of opinionated defaults with targeted overrides.
