---
title: aro-private-dns-zone-resource-removal
authors:
  - "@jim-minter"
reviewers:
  - TBD
approvers:
  - TBD
creation-date: 2021-02-15
last-updated: 2021-02-15
status: provisional
see-also:
  - "/enhancements/dns/plugins.md"
replaces:
superseded-by:
---

# ARO private DNS zone resource removal

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This proposal covers removing the Azure private DNS zone resource from the
architecture of Azure Red Hat OpenShift clusters.

## Motivation

Today on Azure, OCP (and hence ARO) clusters use an Azure private DNS zone to
resolve {api,api-int,\*.apps}.*cluster domain*.  For this to work, it is a
requirement that the Azure vnet in which the VMs are provisioned be configured
to use the default Azure DNS resolver.  This is problematic in ARO for multiple
reasons:

1. Customer DNS configuration errors are a cause of toil and unactionable
   incidents for ARO SREs.  DNS configuration errors also block cluster
   upgrades.

   ARO customers expect to be able to reconfigure vnet DNS and regularly do.
   The ARO service can neither prevent this nor roll back a customer
   configuration change because it may adversely affect non-ARO workloads
   running in the same vnet.

   Any process which involves restarting an ARO VM is dependent on DNS
   configuration to be correct.  If DNS is misconfigured, VMs which restart will
   fail to rejoin the cluster because they cannot resolve api-int.*cluster
   domain*.  Impacted processes include VM upgrade, VM configuration change,
   cluster scale up, VM crash recovery, cluster cold start, manual VM restart,
   etc.

   ARO monitors cluster node readiness and automatically creates an incident
   when a cluster node is not ready.  If the root cause is DNS misconfiguration,
   the service is dependent on communicating with the customer and the customer
   resolving the problem.  This is an expensive process.  During this time the
   incident is unactionable and key service processes are blocked.

1. ARO customers regularly want to configure their clusters to use custom DNS
   servers.  They can use [OCP DNS
   forwarding](https://docs.openshift.com/container-platform/4.6/networking/dns-operator.html#nw-dns-forward_dns-operator)
   to achieve this from pod context, but this does not work from the node
   context and hence cannot be used to resolve on-premise container registries.
   It also represents additional configuration load that the customer must
   carry.

   Some customers need to resolve on-premise registries with wildcard DNS names,
   preventing the use of MachineConfig/DaemonSet workarounds which would modify
   /etc/hosts.

1. ARO intends to enable zero Internet egress by default for ARO clusters.  It
   is likely that in order to implement this, ARO will need to be able to
   selectively override DNS resolution of certain domains in both node and pod
   contexts.

### Goals

* ARO cluster functionality should not be dependent on Azure vnet DNS
  configuration.  Azure private DNS zone resources should not be part of the ARO
  architecture.  The changes required are implemented in ARO, not OCP.

### Non-Goals

## Proposal

ARO would like OCP's approval to remove the private DNS zone resource from its
cluster architecture.  Instead, we propose to resolve
{api,api-int,\*.apps}.*cluster domain* DNS addresses via an inline dnsmasq proxy
resolver running as a systemd service on each cluster VM.  The dnsmasq proxy
resolver would be backed by whichever DNS server(s) the customer specifies in
their vnet configuration.

A working PoC-quality implementation of this approach can be found at
https://github.com/Azure/ARO-RP/pull/1296.  This implementation is notably
similar to the following [Machine Config Operator
code](https://github.com/openshift/machine-config-operator/blob/master/templates/common/on-prem/files/coredns-corefile.yaml),
however by using dnsmasq which is already present in the RHCOS image, it does
not depend on the ability to pull container images.  This will be valuable when
implementing zero Internet egress.

We do not believe that the PoC implementation is trivially applicable to the
OpenShift installer because it is dependent on calculating the IP addresses for
{api,api-int,\*.apps}.*cluster domain* before the installer graph is created.
We suppose this might imply larger installer architecture changes in the
OpenShift installer IPI general case were this ever to be implemented there, but
it is relatively straightforward in ARO.

We do believe that the architectural changes/constraints placed on OCP could be
feasible.  These are:

* ARO clusters would no longer have associated Azure private DNS zones.

* Dnsmasq would be used as an inline proxying DNS resolver for all ARO cluster
  node-context and pod-context DNS resolution.

* Dnsmasq would need to continue to be provided in the RHCOS image.

* ARO VMs would no longer be DNS resolvable by their hostname from other VMs.
  Note that because --kubelet-preferred-address-types is set to InternalIP in
  the Kubelet config, we don't believe this is currently a hard platform
  requirement.

* ARO machine config server URL and serving certificate would use api-int IP
  address instead of DNS.  This is because DNS resolution of api-int would not
  be available at ignition time.

* The ARO cluster installation process would prepopulate the
  openshift-ingress/router-default LoadBalancer service early with the
  appropriate IP address.

* Ingress operator dynamic DNS would be disabled on ARO via overridden
  configuration of the DNS custom resource.

### Risks and Mitigations

One downside of the proposed solution is that it makes it somewhat harder,
although not infeasible, to handle the case of cluster recovery if the api-int
IP address changes.  This is not expected to be a likely occurrence.  ARO
already protects against this case by preventing customers from manually
modifying the configuration of the Azure cluster load balancer resources.  ARO
would be responsible for automatically handling *.apps IP address changes (also
unlikely) as well as defining a process to recover from api-int IP address
changes if ever needed.

Removing the ability to resolve VM hostnames via DNS could conceivably impact
end user workloads, but the likelihood of this is considered low.  We don't
consider the ability to resolve VM hostnames via DNS part of the ARO platform
contract.

## Design Details

{api,api-int,\*.apps}.*cluster domain* IPs are precalculated before any cluster
VMs are created and are considered largely static.  Ignition configs are updated
such that dnsmasq is configured, started and placed inline in /etc/resolv.conf
before any local services that need to resolve those addresses.

### Test Plan

### Graduation Criteria

### Upgrade / Downgrade Strategy

It is believed to be possible to upgrade ARO clusters automatically from their
existing architecture to the proposed architecture without downtime.  After
updated MachineConfigPools are rolled out, the private dns zone can be removed.

### Version Skew Strategy

Version skew strategy is believed not to be applicable.

## Implementation History

## Drawbacks

## Alternatives
