---
title: oci-image-volume
authors:
  - bitoku
reviewers:
  - haircommander
  - saschagrunert
  - kannon92
approvers: # A single approver is preferred, the role of the approver is to raise important questions, help ensure the enhancement receives reviews from all applicable areas/SMEs, and determine when consensus is achieved such that the EP can move forward to implementation.  Having multiple approvers makes it difficult to determine who is responsible for the actual approval.
  - mrunalp
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - None
creation-date: 2025-05-07
last-updated: 2025-05-07
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/OCPNODE-2929
  - https://issues.redhat.com/browse/OCPNODE-2575
see-also:
  - N/A
replaces:
  - N/A
superseded-by:
  - N/A
---
# OCI Image Volume

## Summary

In Kubernetes 1.33, support for OCI Image Volume went to Beta.
This enhancement is to enable this feature in OpenShift.
See [KEP-4639: Summary](https://github.com/kubernetes/enhancements/tree/master/keps/sig-node/4639-oci-volume-source#summary)

## Motivation

See [KEP-4639: Motivation](https://github.com/kubernetes/enhancements/tree/master/keps/sig-node/4639-oci-volume-source#motivation)

### User Stories

See [KEP-4639: User Stories](https://github.com/kubernetes/enhancements/tree/master/keps/sig-node/4639-oci-volume-source#user-stories-optional)

### Goals

- Enable ImageVolume feature gate for OpenShift.

### Non-Goals

- This proposal does not aim to replace existing VolumeSource types.
- This proposal does not address other use cases for OCI objects beyond directory sharing among containers in a pod.
- Enabling on windows containers

## Proposal

1. Enable ImageVolume feature gate in 4.20

- This feature gate is disabled by default in Kubernetes 1.33, so we need to explicitly enable it for OCP

2. Separate Graduation Approach between OCI Images and OCI Artifacts.

Upstream controls both with the same feature gate, but we will take a separate graduation approach in OCP due to the immaturity of OCI artifacts.
To separate graduation, we will add a new CRI-O option to toggle the OCI artifact mount feature rather than introducing a second feature gate.

- **OCI Image Mount**: Promote to GA
- **OCI Artifact Mount**: Begin as DevPreview
  - Disable OCI artifact mount by default

### Workflow Description

#### OCI Image

Since OCI image mount will be GA in 4.20. No feature gate configuration is needed by users. Creating a pod with volumes.image will pull the image and mount in the container.

```
apiVersion: v1
kind: Pod
metadata:
  name: pod
spec:
  containers:
  - name: test
    image: registry.k8s.io/e2e-test-images/echoserver:2.3
    volumeMounts:
    - name: volume
      mountPath: /volume
  volumes:
  - name: volume
    image:
      reference: quay.io/crio/artifact:v1
      pullPolicy: IfNotPresent
```

#### OCI Artifact

While OCI artifact is in dev preview, it fails to mount if a user specifies OCI artifact in reference.
To enable the mount, users need to add this drop-in CRI-O configuration.
This is only done via MachineConfig, and it makes the cluster unsupported.

```
[crio.image]
oci_artifact_mount_support = true
```

When it's enabled, users can mount OCI artifacts in the same way as OCI images.
Once we feel comfortable with the CRI-O's implementation and make it GA, we'd remove this option.

### API Extensions

No API change is needed on the OCP side, but we need to enable ImageVolume feature gate in [features/features.go](https://github.com/openshift/api/blob/master/features/features.go)

### Topology Considerations

#### Hypershift / Hosted Control Planes

N/A

#### Standalone Clusters

N/A

#### Single-node Deployments or MicroShift

N/A

### Implementation Details/Notes/Constraints

To separate graduation of OCI image and OCI artifact, we are going to add a new option to disable OCI artifact in CRI-O and disable OCI artifact mount by default via MCO.

### Risks and Mitigations

See [KEP-4639: Risks and Mitigations](https://github.com/kubernetes/enhancements/tree/master/keps/sig-node/4639-oci-volume-source#risks-and-mitigations)

For OCP, there is a risk to enable the feature gate which is off by default in Kubernetes.
The mitigation is:

- CRI API is easier to change if needed since it's internal to the implementation
- [The Kubernetes API for this feature](https://github.com/kubernetes/enhancements/tree/master/keps/sig-node/4639-oci-volume-source#kubernetes-api) is unlikely to change as it's already in beta and is built on a very stable volume API

### Drawbacks

## Alternatives (Not Implemented)

See [KEP-4639: Alternatives](https://github.com/kubernetes/enhancements/tree/master/keps/sig-node/4639-oci-volume-source#alternatives)

- For graduation of OCI artifacts, we could introduce a new field in ContainerRuntimeConfig to toggle it instead of just removing the option.

## Open Questions \[optional\]

## Test Plan

Although [upstream e2e tests](https://github.com/kubernetes/enhancements/tree/master/keps/sig-node/4639-oci-volume-source#e2e-tests) exist,
they are not run on OCP clusters. We should implement equivalent tests.

## Graduation Criteria

### Dev Preview -> Tech Preview

### Tech Preview -> GA

#### OCI Image

- Complete user documentation
- Comprehensive e2e tests in both CRI-O and OCP

#### OCI Artifact

- Complete user documentation
- Collect use cases and feedback
- Comprehensive e2e tests in both CRI-O and OCP

### Removing a deprecated feature

N/A

## Upgrade / Downgrade Strategy

No OCP-specific differences from [KEP-4639: Upgrade / Downgrade Strategy](https://github.com/kubernetes/enhancements/tree/master/keps/sig-node/4639-oci-volume-source#upgrade--downgrade-strategy)

## Version Skew Strategy

No OCP-specific differences from [KEP-4639: Version Skew Strategy](https://github.com/kubernetes/enhancements/tree/master/keps/sig-node/4639-oci-volume-source#version-skew-strategy)

## Operational Aspects of API Extensions

There are no API extensions.

## Support Procedures

- **OCI Artifact Mount Issues**: By default, artifact mounting is disabled. Check CRI-O configuration and logs to verify configuration status.
- **OCI Image Mount Failures**: If a pod with an OCI image mount fails to create, check the pod status for mount-related errors. Detailed information should be available in CRI-O logs.

## Infrastructure Needed \[optional\]

N/A  
