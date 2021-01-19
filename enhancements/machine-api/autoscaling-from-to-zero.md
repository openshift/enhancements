---
title: Autoscaling from/to zero
authors:
  - @enxebre
reviewers:
  - @frobware
  - @bison
  - @JoelSpeed
  - @alexander-demichev
  - @elmiko
  - @michaelgugino
approvers:
  - @bison

creation-date: 2020-23-01
last-updated: 2020-23-01
status: implementable
see-also:
replaces:
superseded-by:
---

# Autoscaling from/to zero

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift/docs


## Summary

Enable the autocaler to scale from/to zero when using the machine API provider.

## Motivation

Let autocaling through the machine API to be more cost effective.

### Goals

- Enable the autoscaler to scale from/to zero when using the [machine API provider](https://github.com/openshift/kubernetes-autoscaler/tree/master/cluster-autoscaler/cloudprovider/openshiftmachineapi)

- Provide a solution that enables AWS/Azure/GCP particularly but also extends to let any other provider implementation (e.g Openstack, libvirt, baremetal) to expose the details needed by the autoscaler to scale from/to zero.


### Non-Goals

- Support for [node autoprovisioning](https://github.com/kubernetes/autoscaler/blob/master/cluster-autoscaler/proposals/node_autoprovisioning.md)

## Proposal

- As a user I want to the autoscaler to scale from/to zero so I can save money.

- As a dev I want each provider specific logic to be self contained in a single place.

This proposes to let each actuator to contain the provider specific logic to fetch and expose in a machineSet (with zero replicas) the details expected by the autoscaler to be able to create a "virtual" node and scale from zero.

### Implementation Details

#### Machine API Autoscaler provider

The autoscaler exposes
[TemplateNodeInfo](https://github.com/openshift/kubernetes-autoscaler/blob/253ee49441750815c70b606f242eb76164d9bdc4/cluster-autoscaler/cloudprovider/cloud_provider.go#L152)
on the `NodeGroup` interface to let provider implementers to expose
the information of an empty (as if just started) node. This is used in
scale-up simulations to predict what would a new node look like if a
node group was expanded.

- The [machine API autoscaler provider](https://github.com/openshift/kubernetes-autoscaler/tree/master/cluster-autoscaler/cloudprovider/openshiftmachineapi) `nodegroup` implementation operates over scalable resources i.e `machineSets`.

- We want to enable any provider to expose in a machineSet any the details needed by the autoscaler to scale from zero.

- The autoscaler can then use this details to populate a `schedulernodeinfo.NodeInfo` object enabling scaling from zero. To this end we propose the following annotations for a machineSet:

```text
machine.openshift.io/vCPU
machine.openshift.io/memoryMb
machine.openshift.io/GPU
machine.openshift.io/maxPods
```

- Details which are non provider specific can be inferred directly from the [machineSet machine template](https://github.com/openshift/machine-api-operator/blob/master/pkg/apis/machine/v1beta1/machineset_types.go#L80) e.g labels, taints.

- The `TemplateNodeInfo` implementation will look very similar to
  [AWS](https://github.com/openshift/kubernetes-autoscaler/blob/253ee49441750815c70b606f242eb76164d9bdc4/cluster-autoscaler/cloudprovider/aws/aws_cloud_provider.go#L321)/[Azure](https://github.com/openshift/kubernetes-autoscaler/blob/253ee49441750815c70b606f242eb76164d9bdc4/cluster-autoscaler/cloudprovider/azure/azure_scale_set.go#L598)
  autoscaler implementations: but `buildNodeFromTemplate` will get
  what it needs from the given `nodeGroup`/`machineSet` annotations.

- If the autoscaler can't find the relevant annotations mentioned above, it will keep preventing the node group to be scaled to zero and log an error if the `machine.openshift.io/cluster-api-autoscaler-node-group-min-size` annotation is equal to 0.

- OCP defaults to 250 maxPods
  https://github.com/openshift/machine-config-operator/blob/1cb73f2c059788573ce32e951adb4ca2295e2479/templates/worker/01-worker-kubelet/_base/files/kubelet.yaml#L18. We'll
  let the machine API autoscaler provider to lookup the
  `machine.openshift.io/maxPods` annotation and default to 250 if it's
  not found. For customised environments such as the user created its
  own kubelet config with a different maxPods number to be consumed by
  a machineSet, they can set the `machine.openshift.io/maxPods`
  appropriately on the same machineSet so the autoscaler will see that
  number.

#### Providers implementation

- A new controller is developed in each actuator and run by the machine controller manager.

- It watches machineSets and annotate them.

- Is a per provider choice to decide how to infer the details to be included in the well known annotations.

- For AWS/Azure/GCP the details can be inferred from the instance type set in `machineSet.template.Spec.ProviderSpec`. This can be done dynamically or statically [reusing the autoscaler specific provider implementation](https://github.com/openshift/kubernetes-autoscaler/blob/253ee49441750815c70b606f242eb76164d9bdc4/cluster-autoscaler/cloudprovider/aws/ec2_instance_types.go#L30).

- Providers where there's no instance type semantic (Libvirt, Openstack, baremetal) can decide how best infer their details and use the same annotations mechanisim to expose them.

### Risks and Mitigations

- Changing current behaviour for exisisting clusters. Although this
  will introduce support for scaling from/to zero in already running
  autoscaled clusters, this behaviour is opt-in by setting
  `MinReplicas=0`
  https://github.com/openshift/cluster-autoscaler-operator/blob/master/pkg/apis/autoscaling/v1beta1/machineautoscaler_types.go#L15. Current
  `MinReplicas` values must be still honoured by the
  provider. Therefore the new feature should be transparent for
  existing machineAutoscaler resources.

## Design Details

### Test Plan

- Unit testing per provider.

- Unit testing for the machine API autoscaler provider.

- Provider agnostic e2e testing coverage in [machine API e2e test suite](https://github.com/openshift/cluster-api-actuator-pkg/tree/a1c4e0f038c06794b7f1436975a7b1b330317c25/pkg/e2e/autoscaler).


### Graduation Criteria

### Upgrade / Downgrade Strategy

- This new controller will be included in the `cluster-api-actuator-*` image managed by the CVO and exposed by the machine API Operator.

- As this new controller gets deployed into existing clusters, it will automatically annotate and enable scaling from/to zero on all the existing scalable resources targeted by the autoscaler. Though they will still honour `machine.openshift.io/cluster-api-autoscaler-node-group-min-size`.

### Version Skew Strategy

## Implementation History

## Drawbacks

## Alternatives

The [machine API autoscaler provider](https://github.com/openshift/kubernetes-autoscaler/tree/master/cluster-autoscaler/cloudprovider/openshiftmachineapi) could contain the specific logic for each provider. This would defeat one of the main benefits of the machine API which let all the provider implementation details belong to one single place and keep consumers provider agnostic.

## Infrastructure Needed
