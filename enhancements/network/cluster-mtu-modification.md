---
title: Cluster MTU Update

author:

- "@mmahmoud"

reviewers:

- "@bbennett"
- "@danwinship"
- "@dcbw"
- "@squeed"
- "@trozet"

approvers:

- TBD

creation-date: 2021-04-12
status:
---

# Cluster MTU Update

## Summary

Customers need to modify existing cluster's MTU to enable Jumbo frames, or to
fix incorrectly provisioned MTU value.

## Motivation

Customer might want to enable Jumbo frames or fix incorrectly provisoned MTU.

## Goals

- Allow customer to change cluster MTU using kubernete's native way "CRD".

- Customers will have the ablility to increase or decrease MTU value.

- no nodes or pods reload will be required for the MTU to take effect.

- when new nodes comes up they should run with the new MTU setting and not use
  the cluster original default settings.

## Non goals

- Node's physical NIC's MTU will be handled by the customer (is that ok?)
- there will be networking imapact while MTU changes takes effect, this
  will be expected and documeneted behaviour, ideally if we can estimate
  for how long but this really depends on k8s event handling, cluster size.

## Proposal

The following diagram shows all places where MTU update need to take effect

![ovn-k8s-node-mtu](./ovn-k8s-node-MTU.png?raw=true "ovnkube layout")

- MTU setting done by multiple components, MCO which statically handle pyhsical
  NIC and br-ex while ovnkube handles the rest.

- New CRD will be used to configure cluster wide parameters, it will be used for
  MTU settings but in future releases can be extended to have additional
  configuration options, the new CRD will be upstream for the community to use.

- In downstream CNO will be managing this new CRD and use new annotation as a
  mechanism to communicate MTU updates to the rest of the cluster, ovnkube will
  watch for MTU changes and modify the MTU setting on the different interfaces.

- ovnkube need to implement new gRPC API to talk to CRIO to find all pods in all
  namespaces/node, so we can update the POD's veth MTU.

- Now br-ex also needs MTU update today this has been managed by MCO which only
  take effect when the node comes up, so one option is ovnkube implement new
  utility to invoke NetworkManager commands so we can use to change br-ex MTU
  this isn't clean and introduce unneccessry coupling between ovnkube and Network
  Manager, more cleaner approach will require MCO capability to handle dynamicconfiguration
  however MCO team don't have this requirement on their near future roadmap.

## Test plan

- Unit tests for the feature
- e2e tests covering the feature
