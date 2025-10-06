
---
title: microshift-coredns-hosts
authors:
  - eslutsky
reviewers:
  - pacevedom, MicroShift contributor
  - pmtk, MicroShift contributor
  - copejon, MicroShift contributor
approvers:
  - jerpeter1, MicroShift principal engineer
api-approvers:
  - None
creation-date: 2025-10-06
last-updated: 2025-10-06
tracking-link:
  - https://issues.redhat.com/browse/OCPSTRAT-2206
---

# MicroShift  CoreDNS `hosts` Plugin Integration

## Summary

MicroShift relies on CoreDNS to provide DNS resolution for workloads running inside pods within the cluster. In these environments, it is often necessary for applications running in pods to resolve custom, fixed hostnames (such as the local machine's hostname or external services not resolvable via standard DNS) using a **hosts file mechanism**—functionality analogous to `/etc/hosts`, but applied to DNS queries originating from within pods.


## Motivation

The current CoreDNS configuration does not include the **`hosts` plugin**, which prevents users from defining custom, static IP-to-hostname mappings that are honored by the cluster's DNS resolver. Introducing this plugin enhances MicroShift's flexibility, allowing deployments to accommodate use cases requiring custom hostname resolution without relying on complex external DNS services .

### Goals

1.  Integrate the standard **CoreDNS `hosts` plugin** into the MicroShift configuration.
2.  Configure the `hosts` plugin to load static host mappings from a configuration source within the CoreDNS container.
3.  Ensure that changes to the hosts file are reflected immediately, without requiring any service or pod restarts or causing disruption to DNS services.

### Non-Goals

1.  Removing or replacing any existing CoreDNS plugins (e.g., `kubernetes`, `forward`).


## Proposal

The proposal is to modify the MicroShift manifest that deploys CoreDNS (specifically the Corefile configuration) to include the `hosts` plugin. The hosts file referenced by CoreDNS will be an external file, managed directly by MicroShift cluster administrators—MicroShift itself does not author or control the contents of this file. Instead, MicroShift acts as a broker: it monitors the administrator-managed hosts file and ensures its contents are synchronized into a ConfigMap that is mounted into the CoreDNS pods. This approach allows any changes made by cluster admins to the hosts file to be automatically and immediately reflected in CoreDNS, ensuring that custom hostname mappings are always up to date and available to workloads, without requiring manual intervention or pod restarts.

The Corefile configuration will be updated to include a section for the `hosts` plugin and pointing to a hosts file mounted into the CoreDNS Pod.

**Example Corefile snippet (Conceptual Change):**

```corefile
. {
    ...
    # New hosts plugin entry:
    hosts /tmp/hosts/hosts {
        fallthrough
    }        

    reload
    
}
```
The `hosts` plugin is placed at the end of the plugin order to ensure that the default OpenShift service resolution is not overridden or disrupted; CoreDNS will only consult the custom hosts file if the standard resolution mechanisms do not provide an answer.


###  hosts file watcher service
A new service, called the **hosts file watcher**, is introduced to MicroShift. This service is automatically started by MicroShift if the `dns.hosts.status` feature is enabled in the configuration. 

Its primary responsibility is to monitor a specified hosts file (by default `/etc/hosts`, but configurable) for any changes and synchronize its contents into a MicroShift ConfigMap within the `openshift-dns` namespace. This ConfigMap is then mounted into the CoreDNS pods, enabling the CoreDNS `hosts` plugin to serve custom host-to-IP mappings in real time.

###  Propagation Delay
After making changes to the underlying hosts file (whether the default `/etc/hosts` or a custom path), administrators should be aware that it can take **up to 60-90 seconds** for the changes to be propagated and reflected within CoreDNS pods. This delay is primarily due to the Kubelet's sync period and the TTL (time-to-live) configured on the ConfigMap that delivers the hosts file to the CoreDNS pods.

During this propagation window, pods may not immediately resolve hostnames added, removed, or modified in the hosts file. No service restart is required for synchronization; propagation will occur automatically.


### Workflow Description

**MicroShift** is the MicroShift main process.

1. MicroShift starts up.
2. MicroShift loads its configuration. If the hosts feature is not enabled, no further action is taken. If enabled, continue to the next step.
3. MicroShift launches a service that monitors the specified hosts file (default: /etc/hosts).
4. The contents of the hosts file are synchronized to a ConfigMap in the openshift-dns namespace.
5. MicroShift ensures that CoreDNS pods mount this ConfigMap.
6. Any manual changes made to the hosts file are automatically propagated to the CoreDNS service in real time.


### API Extensions
The following changes in the configuration file are proposed:
```yaml
dns:
  hosts:
    status: <Enabled|Disabled>
    file: <filepath>
```
By default, the `dns.hosts.status` feature is **Disabled**. If a Admin enables this feature (i.e., sets `dns.hosts.status` to `Enabled`) but does not specify a file, MicroShift will automatically default `dns.hosts.file` to "/etc/hosts".

### Topology Considerations
#### Hypershift / Hosted Control Planes
N/A

### User Stories
N/A

#### Standalone Clusters
N/A

#### Single-node Deployments or MicroShift
Enhancement is intended for MicroShift only.

### Implementation Details/Notes/Constraints
https://github.com/openshift/microshift/pull/5491

### Risks and Mitigations

### Drawbacks
N/A

## Test Plan
## Graduation Criteria
N/A

### Dev Preview -> Tech Preview
N/A

### Tech Preview -> GA
- Ability to utilize the enhancement end to end
- End user documentation completed and published
- Available by default
- End-to-end tests

### Removing a deprecated feature
N/A

## Upgrade / Downgrade Strategy
When upgrading from 4.20 or earlier to 4.21, the new configuration fields will remain
unset, causing the existing defaults to be used. (so the hosts values wont be passed to coredns)

## Version Skew Strategy
N/A

## Operational Aspects of API Extensions
N/A

## Support Procedures
N/A

## Alternatives (Not Implemented)
An alternative to this enhancement would be to mount the hosts file directly into the CoreDNS pods as a volume. In this approach, the `/etc/hosts` file (or a custom hosts file) from the host node would be mounted into the CoreDNS container, allowing CoreDNS to reference the latest hosts entries.

However, this method has significant drawbacks:
- **Pod Restart Required:** Any changes to the hosts file would require a restart of the CoreDNS pods to take effect, as the file is only read at pod startup. This disrupts DNS resolution during the restart and does not provide seamless updates.
