---
title: Securing the Machine Config Server
authors:
  - @crawford
reviewers:
  - @darkmuggle
  - @kikisdeliveryservice
  - @michaelgugino
  - @danwinship
  - @cgwalters
  - @russellb
  - @derekwaynecarr
approvers:
  - @darkmuggle
  - @russellb
creation-date: 2021-02-01
last-updated: 2021-02-01
status: provisional
---

# Securing the Machine Config Server

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

The bootstrapping process for new cluster nodes is a tricky problem, primarily due to the fact that a new nodes start from a position of almost zero knowledge. These nodes need to fetch their machine configuration and authenticate themselves with the cluster before they can access most resources and have workloads scheduled. At the same time, a malicious actor pretending to be a new node cannot be allowed to join the cluster. Otherwise, they would be able to get access to sensitive resources and could have workloads and their secrets scheduled to them. Today, we protect against this by preventing cluster workloads from accessing the Machine Config Server, the component of the Machine Config Operator which is responsible for serving Ignition configs, derived from a set of Machine Configs, to new machines. This approach presents a problem though: workloads on OpenShift are not allowed to make use of TCP ports 22623 and 22624, the ports used by the Machine Config Server.

## Motivation

This blanket network policy prevents traffic destined for ports 22623 and 22624, regardless of the source and destination IP addresses. This has the potential to impact customer workloads, but also internal initiatives (e.g. OpenShift on OpenShift is complicated by this restriction).

### Goals

- Remove the port restriction on 22623 and 22624
- Ensure that the node bootstrap flow remains secure

### Non-Goals

- Allow customers to use Machine Configs to provide pre-bootstrap configuration

## Proposal

If it can be ensured that no one is able to access sensitive data through the Machine Config Server, the port restrictions can be safely removed. In order to achieve this, Machine Configs can be split into two groups: pre-node-approved configs and post-node-approved configs. The pre-node-approved configs are ones which do not contain sensitive information and contain just enough to configure new nodes to submit a certificate signing request (CSR) to the API server. The post-node-approved configs contain everything else, including the overwhelming majority of customer-provided Machine Configs. When a new machine is created, it fetches its Ignition config from the Machine Config Server, which only serves the pre-node-approved configs. At this point, the machine has enough information to submit a CSR, but nothing sensitive (note that for the purposes of this proposal, the CSR bootstrap token and pull secret are not considered sensitive). Once the CSR has been approved and a kubeconfig returned, the Machine Config Daemon is able to directly enumerate Machine Configs, including the post-node-approved ones, so it then applies any remaining configuration to the machine and reboots if necessary. At this point, the machine is fully configured.

Certain, exotic use cases may require control over which Machine Configs are considered pre-node-approved and post-node-approved. To facilitate those use cases, the Machine Config CRD will need to be updated to include a new boolean field: `neededBeforeNodeApproved`. Since this defaults to `false`, all customer-provided Machine Configs will be placed into the post-node-approved set after the cluster has been updated.

### User Stories

#### Story 1

#### Story 2

### Implementation Details/Notes/Constraints [optional]

The Machine Config Operator will begin rendering two sets of Machine Configs: pre- and post-node-approved; the former being a strict subset of the latter. This means that the post-node-approved rendered Machine Configs will be identical to the rendered Machine Configs today. This is the set of rendered Machine Configs that the Machine Config Daemon will use to reconcile. The pre-node-approved Machine Configs, on the other hand, will only be used by Ignition and they will be to configure the machine to the point that the kubelet can successfully submit a CSR. Because these configs are served without any authentication, care must be taken to ensure that no sensitive information is contained within.

### Risks and Mitigations

- If we make this change to existing clusters, new nodes that are provisioned will no longer include configuration specified in customer-provided Machine Configs until after the CSR has been approved. There is a chance that clusters would fail to auto-scale after applying this update, but that should be very rare. In order for a machine to fetch its Ignition config, it needs to be able to communicate with the API server, specifically, the Machine Config Server. If that's possible, it should also be possible for the kubelet to communicate with that same host.

- This effectively doubles the number of rendered Machine Configs that need to be managed. This might be a concern for clusters with a huge number of Machine Config Pools, but this also seems rare.

- This will introduce a second reboot into the provisioning process in cases where there are post-node-approved Machine Configs. Unfortunately, this is likely to be the case in the environments which are most impacted by reboot times (i.e. bare metal). The second reboot comes from the fact that `machine-config-daemon-firstboot.service` needs to run before the kubelet can start, and this service reboots the machine because it updates the RHCOS installation. After that reboot, the kubelet starts, submits a CSR, and then after approval, the Machine Config Daemon reconciles the post-node-approved Machine Configs and then drains and reboots. We could potentially avoid the reboot by teaching `machine-config-daemon-firstboot.service` to live pivot the machine, allowing the kubelet get its kubeconfig, and then letting the Machine Config Daemon do its reconciliation pass before rebooting. It's not clear to me how feasible this would be.

## Design Details

### Open Questions

1. Can we avoid the extra reboot when post-node-approved Machine Configs are used?

### Test Plan

### Graduation Criteria

#### Dev Preview -> Tech Preview

#### Tech Preview -> GA

#### Removing a deprecated feature

We aren't planning on following a deprecation cycle for this change. We could make use of CRD versioning and phase this in, but the underlying driver for this change is the removal of the TCP port restriction and that wouldn't happen until the deprecation is complete. This timeline would very likely be unacceptable to the customer.

### Upgrade / Downgrade Strategy

Existing machine configs are not modified, including rendered configs, so downgrades will be straightforward (for the Machine Config Operator), though they will leak rendered Machine Configs. The Machine Configs that are created by the Machine Config Operator would be placed into the pre-node-approved set, while any customer-provided Machine Configs would be placed into the post-node-approved set. This ensures that no configuration is inadvertently exposed as the result of an upgrade.

This runs the risk of breaking auto-scaling because certain Machine Configs are not included in the generated Ignition config. Short of selectively-enforcing the network policy, there's really nothing that can be done that doesn't compromise the security of the clusters. We will have to communicate this change to customers since anyone using custom Machine Configs will likely be impacted in some way or another.

### Version Skew Strategy

## Implementation History

## Drawbacks

These have been enumerated in other parts of the proposal, but to get them all in one place:

  - This will negatively impact the scale-up time of clusters which make use of post-node-approved Machine Configs.
  - This has the potential to break a cluster's ability to scale up.

## Alternatives

  - Do nothing and just remove the network policy. This would expose the contents of (what would be) post-node-approved Machine Configs to everything that has network access to the control plane.
  - Add support for an authentication token to the Machine Config Server (https://github.com/openshift/enhancements/pull/443). The real downside to this approach is that it makes troubleshooting more difficult, as it would likely need to happen in the initramfs of RHCOS. This is an extremely limited environment and requires console or serial access, which isn't available in a number of environments.
  - Drop Ignition and the Machine Config Server and pre-bake RHCOS images with the correct contents. This would severely limit deployments that don't make use of the Machine API Operator since the cluster admin would then take on the responsibility of updating the bootimages whenever a change to the Machine Configs is made. This would also take too long to implement to meet the customer's timeline.
