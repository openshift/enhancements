---
title: upi-node-ip-selection
authors:
  - "@pliu"
reviewers: 
  - "@cybertron"
  - "@jcpowermac"
  - "@patrickdillon"
approvers:
  - "@danwinship"
  - "@trozet"
api-approvers: 
  - "None"
creation-date: 2023-02-08
last-updated: 2023-02-16
tracking-link:
  - https://issues.redhat.com/browse/NP-687
see-also:
  - "/enhancements/network/ip-interface-selection"
replaces:
superseded-by:
---

# Node IP Selection UPI

## Summary

When Openshift is deployed on user-provisioned infrastructure, and there are
more than one NIC available on the hosts. We need a reliable way to guarantee
all the openshift components can use the correct IP/interface.

## Motivation

Today, we do not run the nodeip-configuration service on vSphere UPI. It means
we don't have the mechanism to ensure IP/interface selection will return the
same result for following components:

- Node IP (Kubelet and CRIO)
- configure-ovs
- resolv-prepender
- Keepalived
- Etcd

Those components may select different IP/interface as the nodeIP, when there are
multiple NICs in a node, but no default gateway is configured on any of them.

For other UPI platforms, although the nodeip-configuration service is enabled.
It runs in a mode that it will only return the IP/interface with the default
route. The assumption is that the interface with the default route shall have
the nodeIP. However, this assumption is not always true.

Therefore we need to have a generic way for all the UPI platforms. So that users
can specify the node IP/interface selection explicitly.

### User Stories

As a deployer, I want to explicitly specify the UPI cluster machine network to a
subnet without a default gateway when there're more than one interface on my
hosts.

As an cluster admin, after migrating the CNI provider of my existing UPI cluster
from OpenShiftSDN to OVNKubernetes, I need all the openshift components can pick
the correct node IP/interface.

### Goals

- Ensure that all host networked services on a node have consistent interface
and IP selection for UPI platforms.
- Users can specify the network interface at installation at
  install-config.yaml.

### Non-Goals

- Support IPI platforms

## Proposal

Enable the nodeip-configuration service for all the UPI platforms. It will be
the single source for the node ip/interface selection.

### Workflow Description

- At deployment time the cluster creator can specify the `machineNetwork` in the
  install-config.yaml.

- Openshift installer set the `networking.machineNetwork` field of the
  `infrastructures.config.openshift.io/cluster` resource.

- Machine config operator renders the nodeip-configuration.service and set
  `NODEIP_HINT` to the value of `machineNetwork`.

### API Extensions

Enhancement requires below modifications to the mentioned CRDs
- Add `networking.machineNetwork` field to `spec` of the
  `infrastructures.config.openshift.io`
```yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: infrastructures.config.openshift.io
spec:
  versions:
  - name: v1
    schema:
      openAPIV3Schema:
        properties:
          spec:
            properties:
              networking:
                properties:
                  machineNetwork:
                    description: MachineNetwork is the list of IP address pools for machines.
                    items:
                      description: MachineNetworkEntry is a single IP address block for
                        node IP blocks.
                      properties:
                        cidr:
                          description: CIDR is the IP block address pool for machines
                            within the cluster.
                          type: Any
                      required:
                      - cidr
                      type: object
                    type: array
```

### Implementation Details/Notes/Constraints [optional]


### Risks and Mitigations

- Currently all host services are expected to listen on the same IP and
  interface. If at some point in the future we need host services listening
  on multiple different interfaces, this may not work. However, because we
  are centralizing all IP selection logic in nodeip-configuration, it should
  be possible to extend that to handle multiple interfaces if necessary.

### Drawbacks

N/A

## Design Details

- Ask users to specify the `machineNetwork` in the install-config.yaml for UPI
  platforms when there are multiple NICs on the hosts.

- The installer shall populate the value of the `machineNetwork` to the spec
  of the `infrastructures.config.openshift.io/cluster` resource

- Enable the nodeip-configuration.service for all the UPI platforms in the
  machine config operator. When MCO renders the nodeip-configuration.service, it
  shall set the `NODEIP_HINT` variable with the value of the `machineNetwork`.

- Make the baremetal-runtimecfg to be able to select the node ip, when the
  `NODEIP_HINT` variable contains a CIDR list.

If the the cluster creator doesn't configure the `machineNetwork`. The
nodeip-configuration.service will still be enabled. It will run in a no hint
mode, and the interface with default route will be selected.

### Open Questions [optional]

- Shall we also use this mechanism for IPI clusters so that we can have a
  unified code base and improve the maintainability?

### Test Plan

- Add tests for all UPI platforms with `machineNetwork` is specified explicitly.

### Graduation Criteria

N/A

#### Dev Preview -> Tech Preview

N/A

#### Tech Preview -> GA

N/A

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

The cluster admin can choose to manually set the `networking.machineNetwork`
field of the `infrastructures.config.openshift.io/cluster` resource.

### Version Skew Strategy

N/A

### Operational Aspects of API Extensions

N/A

#### Failure Modes

N/A

#### Support Procedures

N/A

## Implementation History

N/A

## Alternatives

Instead of modifying the installer, we can ask user to specify the
`NODEIP_HINT` by following this document
https://docs.openshift.com/container-platform/4.12/support/troubleshooting/troubleshooting-network-issues.html#overriding-default-node-ip-selection-logic_troubleshooting-network-issues.
It means users need to take extra steps before doing the installation and SDN
migration.
