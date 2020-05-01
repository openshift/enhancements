---
title: static-pods-for-single-node-via-ignition
authors:
  - "@deads2k"
reviewers:
approvers:
creation-date: yyyy-mm-dd
last-updated: yyyy-mm-dd
status: provisional|implementable|implemented|deferred|rejected|withdrawn|replaced
see-also:
  - https://github.com/sjenning/rhcos-kaio  
replaces:
superseded-by:
---

# Static Pods for Single Node Cluster via Ignition

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Open Questions [optional]

1. Do we even have a use-case for spending the time building this thought experiment?
2. Do we want to be able to run other operators on top of this?
3. Do we want things like must-gather to function properly?

## Summary

A while back, Seth Jennings had a cool idea for trying to create a single node cluster using ignition.
The cluster would be non-configurable after "creation", non-upgradable, non-HA.
The cluster would only have etcd, kube-apiserver, kube-controller-manager, kube-scheduler.
This is a description of how we could generate supportable static pods.

## Motivation

Documenting a thought experiment about single node clusters.

### Goals

1. Create a supportable static pod.
   Ideally, this looks very similar to our existing static pods.
2. Some amount of configuration is important, how flexible can we be?

### Non-Goals

1. Try to run a kube control plane operator.
   This is unopinionated about what goes on top, but the kube control plane will not be reconfigured after the fact.

## Proposal

We can create a new kind of render command which takes existing inputs *and* config.openshift.io resources.
Similar to how we built the original disaster recovery for certificates, we can factor the command to run the various
control loops "in order".
We can initialize our control loops using fake clients and wire listeners to synthetically update indexers backing fake
listers.
This is like we do for unit tests, only wired into the update reactors for the client.
If we separate the reactive bits of the control loops, the informer watch triggers adn the like, from the data input bits
(I think this is possible), we can have very high fidelity.
In the kube-apiserver, the ordering would like this for instance:
1.  cert-rotation - we need to create certs
2.  encryption - this would need a special mode to say: just encrypt it right away
3.  bound tokens - this creates some secrets for us
4.  static-resources - this creates targets, SAs, and stuff
5.  config observation - we need to set the operator observed config to be able to generate the final config.
6.  target config - writes the kube-apiserver configmap
7.  resource sync - copies bits from A to B
8.  loop through config observation, target config, resource sync one more time (yeah, cycles)
9.  revision controller

Now we do a couple neat things:
1. Export all content from the fake clients to produce resource manifests that will be created bootkube style against
   the kube-apiserver.
   Someone will have grown a dependency and we know for sure that the next operator will require input from the previous one.
2. Wire up the fake clients to our installer command.
   In theory, this command will create an exact copy of the "normal" kube-apiserver static pod that we create.

This gives leaves supporting only one shape of static pods, which makes support of these static pods much easier.


### Restrictions
Some things become impractical once we cannot reconfigure the kube-apiserver, they include...
1. short lifespan of kcm and ksch client certificates - we can no longer rotate these
2. imageregistry causes kube-apiserver reconfiguration - we don't have an operator to manage this
3. authentication reconfiguration - it may be possible to support *some* level of authentication, but without the ability
   to react, options are pretty limited.


