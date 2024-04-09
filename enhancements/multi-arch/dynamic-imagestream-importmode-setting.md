
---
title: dynamic-imagestream-importmode-setting
authors:
  - "@Prashanth684"
reviewers:
  - "@deads2k, apiserver"
  - "@JoelSpeed, API"
  - "@wking, CVO"
  - "@dmage, Imagestreams"
  - "@soltysh, Workloads, oc"
approvers:
  - "@deads2k"
api-approvers:
  - "@JoelSpeed"
creation-date: 2024-04-02
last-updated: 2024-04-02
tracking-link:
  - https://issues.redhat.com/browse/MULTIARCH-4552
---

# Dynamically set Imagestream importMode based on cluster payload type

## Summary

This is a proposal to set ImportMode for
imagestreams dynamically based on the payload type
of the cluster. This means that if the OCP release
payload is a singlemanifest payload, the
importMode for all imagestreams by default will be
`Legacy` and if the payload is a multi payload the
default will be `PreserveOriginal`. 

## Motivation

This change ensures that imagestreams will be
compatible for single and multi arch compute
clusters without needing manual intervention.
Users who install or upgrade to the multi
release payload
(https://mirror.openshift.com/pub/openshift-v4/multi/)
and want to add compute nodes of differing
architectures, need not change the behaviour
of any newly imported imagestreams or any scripts
which import them. For a cluster installed with
single arch payload, imagestreams default import
mode would always be set to `Legacy` because it
is sufficient if the single image manifest is
imported and for a cluster installed with the
multi payload it would be `PreserveOriginal`
which would import the manifestlist in anticipation
that users would add/have already added nodes
of other architectures (which do not match the
control plane's).

### User Stories

- As an Openshift user, I do not want to manually
  change the importMode of imagestreams I want to
  import on the cluster if I add/remove nodes of
  a different architecture to it.
- As an Openshift user, I do not want to change my
  existing scripts and oc commands to add extra
  flags to enable manifestlist imports on
  imagestreams.
- As an Openshift user, I want my imagestreams to
  work with workloads deployed on any node
  irrespective of whether I deploy on a
  single or a multi-arch compute cluster.
- As an Openshift user, I want to be able to toggle
  importMode globally for all imagestreams through
  configuration.

### Goals

- Set the imagestream's import mode based on
  payload architecture (single vs multi) for newly
  created imagestreams.
- Dynamically alter import mode type on upgrade
  from a single arch to a multi payload and vice
  versa (Note: multi->single arch upgrades are
  not officially supported, but can be done
  explicitly).
- Allow users to control the import mode setting
  manually through the `image.config.openshift.io`
  image config CRD.

### Non-Goals

- Describe importmode and what it accomplishes
  relating to imagestreams.
- Existing imagestreams in the cluster will not be
  altered.
- No other imagestream fields are being considered
  for such a change.
- Sparse manifest imports is not being considered.

## Proposal

The proposal is for CVO to expose the
type of payload installed on the
cluster which will be consumed by imageregistry
operator which sets a field in the image config
status and for apiserver to use that information
to set the importMode for imagestreams globally.

### Imagestream ImportModes

The `ImportMode` API was introduced in the 4.12
timeframe as part of the multi-arch compute cluster
project (then called heterogeneous clusters). The
API essentially allowed users to toggle between:
  - importing a single manifest image matching the
    architecture of the control plane (`Legacy`).
  - importing the entire manifestlist (`PreserveOriginal`).

### Workflow Description
 
#### Installation
1. User triggers installation through
openshift-installer.
2. CVO inspects the payload and updates the status
field of `ClusterVersion` with the payload type
(inferred through
`release.openshift.io/architecture`).
3. cluster-image-registry-operator updates the
`imageStreamImportMode` status field based on
whether the image config CRD spec has the
`ImageStreamImportMode` set. If it does, it gets
that value. If not, it looks at the `ClusterVersion`
status and gets the value of the payload type and
based on that, determines and sets the importmode.
4. openshift-apiserver-operator gets the value  of
`ImageStreamImportMode` from the  image config status.
5. the operator updates the `OpenshiftAPIServer`'s
observed config ImagePolicyConfig field with the
import mode.
6. openshift-apiserver uses the value of the
import mode in the observed config's
ImagePolicyConfig field to set the default import
mode for any newly created imagestream.

#### Upgrade
1. User has a cluster installed with a single arch
payload.
2. User triggers an update to multi-arch payload
using `oc adm upgrade --to-multi-arch`.
3. CVO triggers the update, inspects the payload
and updates the status field of `ClusterVersion`
with the payload type.
4. The rest is the same as steps 3-6 in the
install section.

#### Manual
1. User updates the `ImageStreamImportMode` spec
field in the `image.config.openshift.io` `cluster`
CR.
3. cluster-image-registry-operator reconciles the CR
to see if the `ImageStreamImportMode` value is set
and and updates the image config status with the same
value.
4. openshift-apiserver-operator updates the
`OpenshiftAPIServer`'s observed config ImagePolicyConfig
field with the import mode from the image config status.
5. openshift-apiserver uses the value of the
import mode in the observed config's
ImagePolicyConfig field to set the default import
mode for any newly created imagestream.

Note that a change to the import mode in the image
config CR would trigger a redeployment of the
openshift-apiserver

### API Extensions

- image.config.openshift.io: Add
  `ImageStreamImportMode` config flag to CRD spec
  and status.

``` 
type ImageSpec struct {
...
...
// imagestreamImportMode controls the import
// mode behaviour of imagestreams. It can be set
// to `Legacy` or `PreserveOriginal`. The
// default behaviour is to default to Legacy.
// If this value is specified, this setting is
// applied to all imagestreams which do not
// have the value set.
// +optional
ImageStreamImportMode imagev1.ImportModeType
`json:"imageStreamImportMode,omitempty" }`
```
```
type ImageStatus struct {
...
...
// imageStreamImportMode controls the import
// mode behaviour of imagestreams. It can be set
// to `Legacy` or `PreserveOriginal`. The
// default behaviour is to default to Legacy.
// If this value is specified, this setting is
// applied to all new imagestreams which do not
// have the value set.
// +optional
ImageStreamImportMode imagev1.ImportModeType
`json:"imageStreamImportMode,omitempty"
}
```
- openshiftcontrolplane/v1/: Add
  `ImageStreamImportMode` config flag to
  ImagePolicyConfig struct.

```
type ImagePolicyConfig {
...
...
// imageStreamImportMode controls the import
mode behaviour of imagestreams. It can be set
to `Legacy` or `PreserveOriginal`. The
// default behaviour is to default to Legacy.
// If this value is specified, this setting is
// applied to all imagestreams which do not
// have the value set.
// +optional
ImageStreamImportMode imagev1.ImportModeType
`json:"imageStreamImportMode,omitempty"`

}
```
- An example CR for `image.config.openshift.io`
  would look like below if the import mode is set:
```
apiVersion: config.openshift.io/v1
kind: Image
metadata:
  name: cluster
  ...
  ...
spec:
  imageStreamImportMode: PreserveOriginal
status:
   internalRegistryHostname: image-registry.openshift-image-registry.svc:5000
   imageStreamImportMode: PreserveOriginal
  ...
  ...

```
- The ClusterVersion status field CR would look
  like below after CVO has inferred the payload
  type:
```
apiVersion: v1
items:
- apiVersion: config.openshift.io/v1
  kind: ClusterVersion
  spec:
    channel: candidate-4.15
    desiredUpdate:
      force: false
      version: 4.15.0
  status:
    availableUpdates:
    ...
    capabilities:
    ...
    conditions:
    ...
    desired:
      architecture: Multi  # optional, unset/empty-string if it's single arch
    history:
    ...
``` 

### Topology Considerations

#### Hypershift / Hosted Control Planes

- On HyperShift, having the hosted-API-side
ClusterVersion driving control-plane side
functionality(like OpenShift API server behavior)
exposes you to some hosted-admin interference
(see this comment[^1], explaining why we jump through
hoops to avoid touching hosted-API stuff when
setting the upstream update service for HyperShift
clusters). For this particular case, this is not
a problem as the registry lives on the hosted
compute nodes.

#### Standalone Clusters

N/A

#### Single-node Deployments or MicroShift

N/A

### Implementation Details/Notes/Constraints

- CVO: The cluster version operator would expose a
  field in its status indicating the
  type of the payload. This is inferred by CVO from
  the `release.openshift.io/architecture` property
  in the release payload. If the payload is a multi
  payload, the value is `Multi` otherwise it is empty
  indicating a single arch payload.
- Cluster-image-registry-operator: the operator
  sets the `imageStreamImportMode` field in the
  status of the `images.config.openshift.io`
  cluster image config based on whether:
    - `images.config.openshift.io` image config
      has importmode set in its spec.
    - if no global importmode is set in the image
      config, the operator sets it based on the
      payload type (inferred from CVO).Multi would
      mean `PreserveOriginal` and any other value
      would mean `Legacy`(single-arch).
- Cluster-openshift-apiserver-operator: the
  operator sets the import mode value in the
  OpenshiftAPIServer CR's observed config based on
  the `imageStreamImportMode` in the image config
  status.
- Openshift-apiserver: The apiserver  would look
  at the observed config, check the import mode
  value and set the default for the imagestreams
  based on that value.

### Motivations for a new ClusterVersion status property

There were a few options discussed in lieu of
introducing a new ClusterVersion status field
and the potential risks for doing so. The
alternatives are highlighted with reasoning
given for why they were not pursued:
- Default ImportMode to PreserveOriginal
  everywhere: single-arch-release users maybe
  concerned about import size and the lack of
  metadata like `dockerImageLayers` and
  `dockerImageMetadata` for manifestlisted
  imagestream tags.
- Clusters with homogeneous nodes
  running the multi payload who do not
  want to import manifestlists: The clusters
  can either migrate to single arch payloads
  or manually toggle the importMode through
  the image config CRD.
- CVO provides architecural knowledge to
  the cluster-image-registry-operator through
  a configmap or the image config CRD: To
  limit the risk of many external consumers
  using CVO's status field to determine that
  their cluster is multi-arch ready, the idea
  was to expose this information to the specific
  controller. This solution is not necessary as
  we let other controller implementers decide
  if the CVO's new status field is the best fit
  for their use case.

### Risks and Mitigations

- There is a potential growth of space consumption
  in etcd. For each sub manifest present in a manifest
  list, there will be an equivalent Image object with
  a list of its layers and other basic info. While Image
  objects aren't extremely big, this would mean for each
  imagestream tag we will have a few image objects
  rather than just one.
- If imagestreams in a cluster have the `referencePolicy`
  set to `Local`, the images are imported to the cluster's
  internal registry. If many imagestreams follow this pattern,
  there might be space growth in the internal registry.
- Both of the above problems can be mitigated by manually
  inspecting imagestreams and set the importMode to
  `PreserveOriginal` only for the necessary imagestreams or
  change the `referencePolicy` to `Source`.
- Scripts inspecting image objects and imagestream tags to
  access metadata info like `dockerImageLayers` and
  `dockerImageMetadata` will break and will need to change to
  import arch specific tags to get metadata information.

### Drawbacks

- This change would mean users using the multi
  payload would have their imagestream imports be
  manifestlisted by default. For some users this
  might be problematic if they are importing locally
  and would have to deal with mirroring 4x the size
  of the original single manifest image. In this
  case, they might want to change the global to
  `Legacy` and pick and choose which imagestreams
  which would absolutely need to be manifestlisted.

## Open Questions

- How should CVO expose the payload type in the
  Status section? is it through:
    - string value of `single` and `multi` 
    - individual architectures, i.e
      `amd64`/`arm64`/`ppc64le`/`s390x` and
      `multi`?
    - array which lists all architectures for
      `multi` and just one for single arch?
      (in case there is ever a possibility of multiple
      payloads with only certain architectures).

## Test Plan

1. e2e test in openshift-apiserver-operator for
checking the observed config.
2. e2e test in openshift-apiserver.
3. QE test for CVO payload type status reporting
on installs and upgrades.
4. e2e test in the cluster-image-registry-operator
for `imageStreamImportMode` setting in the cluster
image config's status.
4. QE test for installing a cluster on multi
payload and creating imagestreams to check if the
importMode is `PreserveOriginal` by default.
5. QE test for single -> multi and multi -> single
(not supported but can be explicit) upgrades and
confirm appropriate imagestream modes for new
imagestreams.

## Graduation Criteria

- Explicit documentation
  (https://issues.redhat.com/browse/MULTIARCH-4560)
  around this change is necessary to inform users on
  the impact of installing/upgrading to a cluster
  with multi payload.
- implementation of e2e and QE test plans

### Dev Preview -> Tech Preview

N/A

### Tech Preview -> GA

N/A

### Removing a deprecated feature

N/A

## Upgrade / Downgrade Strategy

- For N->N+1 upgrades (assuming N+1 is the first
  release of this feature), the new status field
  will be available in the ClusterVersion

## Version Skew Strategy

- No issues expected with skews

## Operational Aspects of API Extensions

- The `imageStreamImportMode` API setting in the
image config CRD's spec will not be documented. A
cluster-fleet-evaluation[^2] would be done which accesses
Telemetry data to determines the usage. If the field
is not used widely after a set number of releases, deprecate
and remove the field after appropriately informing the
affected customers about the remediation.

## Support Procedures

### Cluster fleet evaluation considerations
- The cluster image registry operator will introduce
  a new `EvaluationConditionsDetected`[^3] condition
  type with a `OperatorEvaluationConditionDetected`
  reason.
- This condition will be set if the `ImageStreamImportMode`
  field is present in the `image.config.openshift.io`
  cluster CR spec. An appropriate message will also be included,
  informing user about this condition.
- Telemetry data will be queried[^4] to determine the number
  of clusters which have this condition prevalent.
- Monitor the telemetry data for x number (four suggested)
  of releases.
- After set number of releases, customers will be alerted[^5]
  of this condition and the remediation through:
  - A KCS article which would recommend the user to remove the
    `ImageStreamImportMode` setting in favor of changing the
    import mode of affected imagestreams directly through
    manual patching or a script.
  - Since we estimate that not many customers might use this
    setting, it will be communicated through email to affected
    customers informing them of the remediation.

## Alternatives

- Manual setting of import mode through editing
  imagestreams or patching.
- If oc commands are used to import imagestreams
  add the `import-mode` flag to set it
  appropriately.

## References
[^1]:https://github.com/openshift/cluster-version-operator/pull/1035#issue-2133730335
[^2]:https://github.com/openshift/enhancements/blob/master/dev-guide/cluster-fleet-evaluation.md
[^3]:https://github.com/openshift/api/blob/master/config/v1/types_cluster_operator.go#L211
[^4]:https://github.com/openshift/enhancements/blob/master/dev-guide/cluster-fleet-evaluation.md#interacting-with-telemetry
[^5]:https://github.com/openshift/enhancements/blob/master/dev-guide/cluster-fleet-evaluation.md#alerting-customers