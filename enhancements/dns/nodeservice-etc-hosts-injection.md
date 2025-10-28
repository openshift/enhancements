---
title: nodeservices-etc-hosts-injection
authors:
  - "@t-cas"
  - "@sdodson"
reviewers:
  - "@Miciah"
  - "@candita"
  - "@alebedev87"
  - "@JoelSpeed"
  - "@everettraven"
approvers:
  - "@Miciah"
  - "@knobunc"
creation-date: 2025-10-27
last-updated: 2025-10-27
status: implementable
see-also:
  - "/enhancements/dns/plugins.md"
replaces:
superseded-by:
---

# Node /etc/hosts Injection for Services

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement provides the ability to configure the cluster DNS operator to inject specific Kubernetes service IP addresses into the `/etc/hosts` file on all cluster nodes. This enables critical cluster services to be resolvable via `/etc/hosts` before DNS resolution is fully operational, particularly during cluster bootstrap and upgrade scenarios.

## Motivation

During cluster operations such as bootstrap and upgrades, certain core services like the cluster image registry need to be resolvable by the container runtime before the DNS infrastructure is fully operational. Currently, the DNS node resolver provides this capability for a hardcoded list of services. This enhancement makes that list configurable, allowing administrators to customize which services are injected into `/etc/hosts` on nodes.

### Goals

1. Provide an API for administrators to specify a list of Kubernetes services that should have their IP addresses injected into `/etc/hosts` on all nodes.
2. Ensure critical services remain resolvable during cluster bootstrap, upgrades, and DNS disruptions.
3. Make the existing hardcoded behavior configurable while maintaining backward compatibility.
4. Support both cluster-scoped and namespace-scoped service resolution.

### Non-Goals

1. Providing a general-purpose mechanism for arbitrary host file entries (only Kubernetes services are supported).
2. Supporting external DNS names or IP addresses that are not associated with Kubernetes services.
3. Replacing DNS resolution as the primary mechanism for service discovery.
4. Managing `/etc/hosts` entries for non-cluster services.

## Proposal

Add a new `nodeServices` field to the DNS operator API that allows administrators to specify a list of Kubernetes services whose IP addresses should be added to `/etc/hosts` on all cluster nodes. The DNS node resolver will watch these services and maintain the `/etc/hosts` file accordingly.

A new field will be added to the `DNS` type in the `operator.openshift.io/v1` API:

```go
// NodeService defines a service that should be added to /etc/hosts on all nodes.
type NodeService struct {
    // namespace is the namespace of the service.
    // This field is required and must not be empty.
    // +kubebuilder:validation:Required
    // +kubebuilder:validation:MinLength=1
    Namespace string `json:"namespace,omitempty"`

    // name is the name of the service.
    // This field is required and must not be empty.
    // +kubebuilder:validation:Required
    // +kubebuilder:validation:MinLength=1
    Name string `json:"name,omitempty"`
}

type DNSSpec struct {
    // ... existing fields ...

    // nodeServices is a list of services that should be added to /etc/hosts
    // on all nodes. This is useful for services that need to be resolvable
    // before DNS is fully operational, such as during cluster bootstrap or
    // upgrades.
    //
    // Each service's ClusterIP will be added to /etc/hosts with an entry
    // in the format: "<ClusterIP> <service-name>.<namespace>.svc"
    //
    // If not specified, a default list of critical services will be used
    // to maintain backward compatibility.
    //
    // +optional
    // +listType=map
    // +listMapKey=namespace
    // +listMapKey=name
    NodeServices []NodeService `json:"nodeServices,omitempty"`
}
```

### User Stories

#### Story 1: Custom Image Registry Resolution

As a cluster administrator managing a custom internal image registry service, I want to ensure that the container runtime on all nodes can resolve my registry's service name even during cluster upgrades when DNS may be temporarily unavailable, so that image pulls for core components can succeed reliably.

Example:
```yaml
apiVersion: operator.openshift.io/v1
kind: DNS
metadata:
  name: default
spec:
  nodeServices:
  - namespace: my-custom-namespace
    name: my-custom-registry
```

#### Story 2: Cluster provided infrastructure services

As a platform team managing critical infrastructure services that need to be available to node scoped processes we want /etc/hosts entries to be populated to provide these entries 

Example:
```yaml
apiVersion: operator.openshift.io/v1
kind: DNS
metadata:
  name: default
spec:
  nodeServices:
  - namespace: coredump
    name: datastore
```

### Implementation Details/Notes/Constraints

#### DNS Node Resolver Implementation

The DNS node resolver (running as a DaemonSet on all nodes) will:

1. The cluster-dns-operator watches the `DNS` custom resource for changes to the `nodeServices` field
2. For each service specified in `nodeServices`, the operator constructs a list of service names, the internal
   registry service `image-registry.openshift-image-registry.svc` is always added to the list
   - Format: Service names in `service-name.namespace.svc` format
   - Example: `datastore.coredump.svc`
3. The operator populates the node-resolver DaemonSet `SERVICES` environment variable with the list of service names
   - Format: Comma-separated and/or space-separated list of service names in `name.namespace.svc` format
   - Example: `SERVICES="image-registry.openshift-image-registry.svc, datastore.coredump.svc"`
   - The script appends `.${CLUSTER_DOMAIN}` when performing DNS lookups to create the full FQDN
4. The DNS node resolver pods read the `SERVICES` environment variable on startup
5. The `update-node-resolver.sh` script runs on each node and:
   - Parses the list of service names from the `SERVICES` environment variable (comma/space-separated)
   - For each service, performs DNS lookups using `dig` to resolve the ClusterIP
     - Tries multiple resolution methods: IPv4, IPv6, and TCP fallback (for environments like Kuryr that don't support UDP)
     - Uses the cluster's nameserver (`${NAMESERVER}`) and cluster domain (`${CLUSTER_DOMAIN}`)
   - Updates `/etc/hosts` with entries in the format: `<ClusterIP> <service-name>.<namespace> <service-name>.<namespace>.<cluster-domain> # openshift-generated-node-resolver`
   - Runs in a 60-second loop to continuously detect and apply IP changes
   - Only updates `/etc/hosts` if valid IPs are resolved (preserves stale entries if DNS/API is unavailable)
   - Uses atomic file operations to prevent corruption
6. When the `nodeServices` list changes in the DNS CR, the operator updates the DaemonSet `SERVICES` environment variable, triggering a rolling restart to pick up the new service list

Steps 1-4 are the only steps modified by this enhancement.

#### Validation

The following validation rules will be enforced:

- `namespace` and `name` fields must not be empty
- Service names must conform to DNS label standards (RFC 1123)
- Namespace names must conform to Kubernetes namespace naming requirements
- Maximum of 20 additional services can be specified
  - This limit is imposed due to the implementation storing service names in the DaemonSet `SERVICES` environment variable
  - Kubernetes has practical limits on environment variable sizes (typically 128KB total for all variables)
  - Each service entry requires storage for the service name in `service-name.namespace.svc` format only (IPs are looked up dynamically)
  - Example: `image-registry.openshift-image-registry.svc` requires approximately 30-70 bytes
  - 20 services × ~70 bytes = ~1.4KB, leaving ample headroom for other environment variables
  - The limit provides a safety margin and aligns with the intended use case (critical infrastructure services)

#### Backward Compatibility

If the `nodeServices` field is not specified or is empty, the DNS operator will use a default list containing the cluster image registry service to maintain backward compatibility with existing clusters. This ensures that existing functionality is preserved during upgrades.

#### File Management

The DNS node resolver will:
- Use atomic file operations (write to temp file, then copy) to prevent corruption
- Preserve the original file's attributes (permissions, ownership)
- Add a comment marker (`# openshift-generated-node-resolver`) to identify managed entries
- Filter out old managed entries before adding updated ones
- Only update `/etc/hosts` if valid service IPs are resolved

#### Environment Variable Considerations

The implementation uses a Kubernetes DaemonSet environment variable named `SERVICES` to pass a list of service hostnames to the DNS node resolver pods. The `update-node-resolver.sh` script then dynamically looks up the IP addresses for these services. This approach has several important implications:

**Advantages:**
- Simple and reliable mechanism built into Kubernetes
- No need for shared storage or complex synchronization
- Standard Kubernetes primitives for configuration management
- **Service IP changes don't require pod restarts**: The script detects and updates IPs dynamically via 60-second polling
- Small footprint: only service names in `service-name.namespace.svc` format are stored, not IP addresses
- Efficient: ~1.4KB for 20 services, minimal impact on environment variable limits
- Flexible format: supports comma-separated, space-separated, or mixed delimiters

**Limitations:**
- **Service list changes require pod restarts**: When services are added to or removed from the `nodeServices` list in the DNS CR, the operator must update the DaemonSet `SERVICES` environment variable, triggering a rolling restart of all node resolver pods
  - Rolling restart respects PodDisruptionBudgets and ensures gradual propagation
  - Brief window where some nodes may be tracking different service lists during rolling update
- **Script-based IP lookup**: The `update-node-resolver.sh` script must:
  - Run in a continuous 60-second polling loop
  - Resolve service IPs via DNS using `dig` command (tries IPv4, IPv6, and TCP fallback)
  - Handle services that don't exist or have no ClusterIP
  - Preserve stale entries if DNS resolution fails temporarily (doesn't remove working entries during outages)
- **Size constraints**: Total environment variable size for a pod is limited (typically 128KB)
  - 20 services limit provides safety margin
  - Each service name in `service-name.namespace.svc` format is approximately 30-70 bytes
  - Leaves ample space for other environment variables
- **Visibility**: Service names are visible in pod specs and environment variables (not sensitive data)

**Update Propagation:**

**For service list changes** (adding/removing services from `nodeServices`):
- The operator updates the DaemonSet `SERVICES` environment variable
- This triggers a rolling update with the following characteristics:
  - Gradual rollout across all nodes (default: 1 pod at a time)
  - Respects node scheduling constraints and taints
  - Typically completes within minutes for most clusters
  - Different nodes temporarily track different service lists during the rollout

**For service IP changes** (ClusterIP of a tracked service changes):
- The `update-node-resolver.sh` script detects the IP change during its next 60-second polling cycle
- Updates `/etc/hosts` on the local node without requiring pod restart
- Detection happens automatically via periodic DNS lookups
- Much more efficient than pod restarts for IP changes
- Maximum detection lag: 60 seconds

#### Security Considerations

- The DNS node resolver runs with elevated privileges (required to modify `/etc/hosts`)
- Only services with valid ClusterIPs will be added (no external IPs or LoadBalancer IPs)
- Service namespace and name validation prevents injection attacks
- The resolver will not add entries that conflict with existing system entries (e.g., localhost)
- The `SERVICES` environment variable contains only service names, not IP addresses
- Service names stored in environment variables are not considered sensitive information
- The `update-node-resolver.sh` script must securely query the Kubernetes API or DNS to resolve service IPs
- RBAC controls which users can modify the DNS CR to change the `nodeServices` list

### Risks and Mitigations

The primary risk involved here is that the list of services becomes long and as a result the DaemonSet and all
pods derrived from it become large. We limit the number of entries to 20 and envision that most clusters will 
utilize only 1 or 2 such entries.

All other risks exist already.

## Design Details

### Open Questions

### Test Plan

#### Unit Tests

- Validation of the `NodeService` struct, that's the only portion updated by this enhancement

#### Integration Tests

- Deploy a DNS operator with custom `nodeServices` configuration
- Verify that DaemonSet `SERVICES` environment variable is populated correctly with service names (not IPs)
- Verify that the `update-node-resolver.sh` script:
  - Parses the `SERVICES` environment variable correctly
  - Looks up service IPs successfully
  - Adds specified services to `/etc/hosts` on nodes with correct IPs
- Verify that service IP changes are detected and applied **without triggering pod restarts**
  - Change a service's ClusterIP
  - Verify that `/etc/hosts` is eventually updated on all nodes
  - Verify that pods did not restart
- Verify that adding/removing services from the `nodeServices` list triggers DaemonSet rolling update
  - Add a new service to the list
  - Verify `SERVICES` environment variable is updated
  - Verify pods restart to pick up new service list
- Verify that service deletions are handled gracefully (entries removed from `/etc/hosts`)

#### E2E Tests

- Deploy a test service with backend
- Add the service to nodeServices
- verify that a host process can resolve and connect to that service

### Graduation Criteria

#### Dev Preview

- API is defined and reviewed
- Basic functionality implemented in cluster DNS operator
- Unit and integration tests passing
- Documentation for the API exists

#### Tech Preview

- Feature is feature-gated
- E2E tests are implemented and passing
- Gather feedback from early adopters
- Performance testing completed
- Monitoring and alerting configured

#### GA

- Feature is proven stable in Tech Preview for at least one release
- All tests consistently pass
- Performance meets requirements (minimal overhead)
- User documentation is complete
- Upgrade/downgrade testing completed
- No major bugs reported in Tech Preview

### Upgrade / Downgrade Strategy

#### Upgrade

- Existing clusters will not have `nodeServices` configured
- Default behavior (image registry injection) will continue to work
- Administrators can optionally configure `nodeServices` after upgrade
- The DNS node resolver will be updated via normal DaemonSet rolling update

#### Downgrade

- If downgrading to a version without this feature, the `nodeServices` field will be ignored
- The old default behavior will resume
- Entries in `/etc/hosts` will be cleaned up by the old version of the DNS node resolver
- No data loss or corruption expected

### Version Skew Strategy

During upgrades, there may be a period where:
- Some nodes are running the new DNS node resolver (understands `nodeServices`)
- Some nodes are running the old DNS node resolver (uses hardcoded list)

This is acceptable because:
- Both versions maintain the critical image registry entry
- The inconsistency is temporary and resolves when the rolling update completes
- Services that depend on these entries are designed to retry on failure

## Implementation History

- 2025-08-04: Initial API proposed in https://github.com/openshift/api/pull/2435
- 2025-08-04: Initial implementation proposed in https://github.com/openshift/cluster-dns-operator/pull/441
- 2025-10-01: Tests passing, awaiting review

## Drawbacks

1. Adds complexity to the DNS node resolver and cluster-dns-operator
2. Service list changes require pod restarts across all nodes
   - Rolling update takes time to propagate new service lists cluster-wide
   - Different nodes temporarily track different service lists during updates
3. Not real-time - IP changes detected based on 60-second polling interval (up to 60 seconds lag)

## Alternatives

### Alternative 1: Use CoreDNS Hosts Plugin

Instead of modifying `/etc/hosts`, configure CoreDNS to use the hosts plugin with a dynamically generated hosts file.

**Pros**:
- No need for elevated privileges on nodes
- Centralized configuration
- Standard CoreDNS functionality

**Cons**:
- Doesn't solve the bootstrap problem (CoreDNS needs to be running)
- Container runtime may not use CoreDNS for resolution
- More complex to implement and maintain

### Alternative 2: Node-local DNS Cache (dnsmasq)

Deploy a node-local DNS cache that has built-in knowledge of critical services.

**Pros**:
- Standard DNS resolution mechanism
- Better performance for all queries

**Cons**:
- Adds another component to maintain
- Doesn't solve the container runtime resolution issue
- More complex architecture

### Alternative 3: ConfigMap Instead of Environment Variables

Use a ConfigMap mounted as a volume in the DNS node resolver pods instead of the `SERVICES` environment variable.

**Pros**:
- No size limitations like environment variables
- ConfigMap updates can be detected without pod restart (when using file watching)
- Could support more than 20 services
- More scalable approach
- Changes to the service list wouldn't require pod restarts

**Cons**:
- Requires implementing file watching mechanism in the `update-node-resolver.sh` script
- More complex synchronization logic
- ConfigMap propagation to nodes takes time (kubelet sync period, typically up to 1 minute)
- Added complexity may not be justified given:
  - Service lists change infrequently
  - The 20-service limit is sufficient for the intended use case
  - Environment variable approach is simpler and more reliable

**Why the `SERVICES` environment variable approach was chosen**:
- **Simplicity** - standard Kubernetes mechanism, no custom logic needed
- **Reliability** - pods get consistent view at startup, no risk of partial updates
- **Sufficient for use case** - 20 services covers critical infrastructure needs
- **Service names only** - storing just names in `service-name.namespace.svc` format (not IPs) means ~1.4KB for 20 services
- **Clear restart semantics** - pod restart clearly signals service list configuration change
- **Single source of truth** - environment variable is set once at pod startup
- **No additional watch logic needed** - the script already polls for IP changes via DNS every 60 seconds
- **Flexible format** - comma/space-separated format is simple to parse and human-readable

Note: Since the `update-node-resolver.sh` script already handles dynamic IP lookups via DNS, the environment variable only needs to contain the service list, making the size constraint very manageable.

## Infrastructure Needed

- None beyond existing OpenShift CI/CD infrastructure
- Standard e2e test environment sufficient for validation

