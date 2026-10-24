---
title: aws-nested-virtualization
authors:
  - "@barbacbd"
reviewers:
  - "@rh-cbent, author of the openshift/api change, for API shape and naming"
  - "@damdo, for Machine API / Cluster API alignment with upstream CAPA"
  - "@JoelSpeed, for Machine API ownership and feature gate lifecycle"
approvers:
  - "@JoelSpeed"
api-approvers:
  - "@JoelSpeed"
creation-date: 2026-10-20
last-updated: 2026-10-20
status: provisional
tracking-link:
  - https://issues.redhat.com/browse/CORS-4686
  - https://issues.redhat.com/browse/CORS-4541
  - https://issues.redhat.com/browse/OCPSTRAT-3494
see-also:
  - "/enhancements/machine-api/dedicated-instances.md"
replaces: []
superseded-by: []
---

# AWS Nested Virtualization in Machine API

## Summary

AWS [supports nested virtualization on virtual (non-bare-metal) EC2 instances](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/amazon-ec2-nested-virtualization.html)
for the C8i, M8i, and R8i instance families. It is set via `CpuOptions.NestedVirtualization` at
instance launch (`RunInstances`). It can also be changed afterwards with `ModifyInstanceCpuOptions`,
but only while the instance is **stopped** — never on a running instance.

Today the only way to get nested virtualization on an OpenShift node is a day-2 workaround: provision
the Machine, then stop the instance, mutate its CPU options out-of-band, and start it again. This
enhancement replaces that with **launch-time, declarative** enablement: the capability becomes part
of the Machine's desired state, so instances come up with it already on and every replacement Machine
inherits it. Install-time (day-0) configuration via `install-config.yaml` is a separate effort and is
a Non-Goal here; see [CORS-4541](https://issues.redhat.com/browse/CORS-4541).

It adds a `nestedVirtualization` field to the existing `cpuOptions` struct in
`AWSMachineProviderConfig` (`machine.openshift.io/v1beta1`) and plumbs it through
`machine-api-provider-aws` to `RunInstances`. The field is guarded by a new OpenShift feature gate,
`AWSNestedVirtualization`, enabled in `TechPreviewNoUpgrade` and `DevPreviewNoUpgrade` only.

**This enhancement is blocked on an SDK migration.** `machine-api-provider-aws` is pinned to
`aws-sdk-go` v1, which does not expose `CpuOptions.NestedVirtualization` and never will — v1 is
archived and out of support. The actuator change is small, but it cannot be written until the
`RunInstances` path moves to `aws-sdk-go-v2`. That migration is a prerequisite for Tech Preview and
needs an owner and sign-off from the Machine API team; see Implementation Details.

The API change is already open as [openshift/api#3088](https://github.com/openshift/api/pull/3088)
and mirrors the equivalent field that has already landed upstream in
[cluster-api-provider-aws](https://github.com/kubernetes-sigs/cluster-api-provider-aws/blob/5ffa11af3b508eff794fba48a673cda2bbed03d6/api/v1beta2/types.go#L618-L659)
(CAPA), so the Machine API and Cluster API representations stay aligned ahead of the MAPI-to-CAPI migration.

## Motivation

Nested virtualization is the peer-pod-free deployment path for OpenShift Sandboxed Containers (OSC):
the Kata VM runs directly on an existing OpenShift worker node rather than in a separate peer-pod VM.
It is also required to run OpenShift Virtualization (CNV) workloads on non-metal instances, which are
roughly 50-60% cheaper than the bare-metal alternatives for CI, dev, and test clusters.

There is no way to express nested virtualization in a MachineSet today, so it cannot be part of a
Machine's desired state. The current practice is a day-2 script
([CNV-86080](https://issues.redhat.com/browse/CNV-86080)) that, for every node needing the capability,
drains it, stops the EC2 instance, mutates its CPU options with AWS CLI v2, and starts it again. That
is disruptive to a running cluster, leaves the Machine API unaware of the instance's real state,
requires EC2 mutate permissions from outside the cluster, and — because it is not part of the
Machine's desired state — is lost on every scale-up, remediation, or MachineSet rollout, which makes
autoscaled node groups unusable.

Declaring the capability in the `providerSpec` instead makes it part of the Machine's desired state:
the instance is launched with nested virtualization already on, nothing is drained or power-cycled,
and every replacement Machine inherits the setting. This is launch-time and declarative, but it is
not yet install-time — an administrator still authors a MachineSet on a running cluster. True day-0
enablement requires the installer work in [CORS-4541](https://issues.redhat.com/browse/CORS-4541) to
surface the setting in `install-config.yaml` and render it into the MachineSets it generates, which
depends on the field added here.

### User Stories

* As a cluster administrator deploying OpenShift Sandboxed Containers, I want to create a MachineSet
  of `m8i` workers with nested virtualization enabled, so that Kata VMs run on the worker nodes
  without a separate peer-pod VM.
* As a cluster administrator running OpenShift Virtualization in a test environment, I want nested
  virtualization on non-metal instances, so that I do not pay for bare-metal nodes.
* As a cluster administrator, I want nested virtualization to survive Machine replacement, so that I
  do not need to re-run a manual AWS CLI workaround after every scale-up or remediation.

### Goals

* Add `cpuOptions.nestedVirtualization` to `AWSMachineProviderConfig`.
* Set `CpuOptions.NestedVirtualization` on `RunInstances` from `machine-api-provider-aws`.
* Introduce the `AWSNestedVirtualization` feature gate and gate the field behind it in the Machine API
  admission webhooks.
* Keep the field's placement and semantics consistent with upstream CAPA so the CAPI migration is a
  mechanical conversion. Enum values follow OpenShift conventions and are PascalCase, so the
  conversion maps values rather than copying them.

### Non-Goals

* Day-2 enablement on existing instances. AWS does permit this on a *stopped* instance via
  `ModifyInstanceCpuOptions`, but no stop/modify/start orchestration is added here; setting the field
  on an existing Machine does not retrofit its instance, so the Machine must be replaced. See
  Alternatives for why.
* Install-time (day-0) enablement. Surfacing the setting in `install-config.yaml` is tracked under
  [CORS-4541](https://issues.redhat.com/browse/CORS-4541), which depends on this field. This
  enhancement delivers launch-time enablement only.
* Cluster API (CAPA) or `cluster-capi-operator` conversion work. CAPA already has the field; the MAPI
  to CAPI conversion is tracked under [OCPCLOUD-3315](https://issues.redhat.com/browse/OCPCLOUD-3315).
* HyperShift NodePool support.
* GCP and Azure. Azure nested virt already works out of the box; GCP is tracked in
  [CORS-4694](https://issues.redhat.com/browse/CORS-4694).
* Validating that the chosen instance type, region, or AMI actually supports nested virtualization.
  AWS rejects the `RunInstances` call and the Machine reports `InvalidMachineConfiguration`.

## Proposal

Add a single optional field to the existing `CPUOptions` struct — the same struct that already carries
`confidentialCompute` — and consume it in the AWS actuator. Gate the field behind a new feature gate
until it has soaked in Tech Preview.

### Workflow Description

**cluster administrator** is a human user responsible for the cluster's compute configuration.

1. The cluster administrator enables the `TechPreviewNoUpgrade` feature set on the cluster.
2. The cluster administrator creates or edits a MachineSet whose `providerSpec` selects a supported
   instance type (C8i, M8i, or R8i) and sets `cpuOptions.nestedVirtualization: Enabled`.
3. The Machine API validating webhook admits the MachineSet because the `AWSNestedVirtualization`
   feature gate is enabled.
4. The machine controller creates a Machine, and `machine-api-provider-aws` calls `RunInstances` with
   `CpuOptions.NestedVirtualization` set.
5. The resulting Node exposes `/dev/kvm`, and nested-virtualization workloads can be scheduled onto it.

If the feature gate is disabled, step 3 fails and the webhook rejects the object with a clear message.

### API Extensions

This enhancement modifies `AWSMachineProviderConfig`, the AWS `providerSpec` embedded in
`machine.openshift.io/v1beta1` `Machine` and `MachineSet` — and also, by virtue of the same embedded
type, in `machine.openshift.io/v1` `ControlPlaneMachineSet`. It adds no new CRDs, webhooks, or
aggregated API servers; it extends the two existing Machine API admission webhooks, and requires a
corresponding rejection rule in the `ControlPlaneMachineSet` webhook owned by
`cluster-control-plane-machine-set-operator` (see Implementation Details).

```go
// +kubebuilder:validation:Enum=Enabled;Disabled;""
type AWSNestedVirtualizationPolicy string

const (
	AWSNestedVirtualizationPolicyDisabled AWSNestedVirtualizationPolicy = "Disabled"
	AWSNestedVirtualizationPolicyEnabled  AWSNestedVirtualizationPolicy = "Enabled"
)

type CPUOptions struct {
	// ConfidentialCompute: existing field, unchanged.
	ConfidentialCompute *AWSConfidentialComputePolicy `json:"confidentialCompute,omitempty"`

	// nestedVirtualization allows hypervisors such as KVM to run inside a virtual
	// (non-bare-metal) EC2 instance. Only available on a subset of instance types.
	// When omitted, this means no opinion and the AWS platform is left to choose a
	// reasonable default, which is subject to change without notice. The current
	// default is Disabled.
	// +optional
	NestedVirtualization AWSNestedVirtualizationPolicy `json:"nestedVirtualization,omitempty"`
}
```

Full godoc is in [openshift/api#3088](https://github.com/openshift/api/pull/3088).

Three deliberate departures from what openshift/api#3088 currently proposes, all of which must be
settled before that PR merges because none can be changed after GA:

**Value type, not a pointer.** `dev-guide/api-conventions.md` advises against pointers for optional
fields in custom-resource-based APIs unless there is an absolute need, and a string enum already has
an unset representation — the empty string — so `omitempty` on a value type is sufficient and
unambiguous. The enum marker therefore includes `""`, following the worked example in the
conventions. The sibling `confidentialCompute` is a pointer, which makes the struct internally
asymmetric; that is accepted deliberately, because the conventions are explicit that "the existence
of non-compliant APIs is not a justification for introducing additional non-compliant APIs." As a
secondary benefit, upstream CAPA also declares these as value types, so this removes a
pointer-to-value step from the MAPI-to-CAPI conversion.

**`AWS`-prefixed type name.** The sibling type is `AWSConfidentialComputePolicy`;
openshift/api#3088 currently declares an unprefixed `NestedVirtualizationPolicy`, copied from CAPA.
`AWSNestedVirtualizationPolicy` matches the surrounding package.

**PascalCase enum values.** CAPA uses lowercase `enabled` / `disabled`; this enhancement uses
`Enabled` / `Disabled`, matching `confidentialCompute` in the same struct and OpenShift conventions
generally. The cost is that the MAPI-to-CAPI conversion maps values rather than copying them. Worth
noting that CAPA is itself inconsistent here — its `AWSConfidentialComputePolicy` is already
PascalCase next to its lowercase `NestedVirtualizationPolicy` — so the divergence is from an upstream
inconsistency, not an upstream convention.

`nestedVirtualization` and `confidentialCompute` are independent fields at the AWS API level, but AMD
SEV-SNP and nested virtualization cannot be used together on the same instance.
[openshift/api#3088](https://github.com/openshift/api/pull/3088) expresses this as a CEL rule on
`CPUOptions`; see [Open Questions](#open-questions-optional) for why that rule is not self-enforcing here.

### Topology Considerations

#### Hypershift / Hosted Control Planes

Not applicable. HyperShift uses CAPA's `AWSMachineTemplate`, not `AWSMachineProviderConfig`, and CAPA
already carries the field. Surfacing it in `NodePool` is separate work under
[OCPSTRAT-3494](https://issues.redhat.com/browse/OCPSTRAT-3494).

#### Standalone Clusters

The target topology, and the only one where the field is meaningful.

#### Single-node Deployments or MicroShift

MicroShift does not run the Machine API and is unaffected. Single-node OpenShift has only control
plane machines, and the webhook rejects this field on control plane machines for Tech Preview (see
Implementation Details), so the feature is unavailable on SNO rather than merely untested. Lifting
that restriction for SNO and compact clusters is deferred to GA and would need the nested-virt
overhead quantified against SNO's resource envelope first, since the control plane and the nested
workload share one node.

#### OpenShift Kubernetes Engine

Available, since OKE includes the Machine API. Nothing OKE-specific to consider.

### Implementation Details/Notes/Constraints

* **openshift/api** — [openshift/api#3088](https://github.com/openshift/api/pull/3088) adds the type,
  constants, and field, and is still open and under review. Four changes are needed before it merges:
  add the feature gate to `features/features.go` and tag the field with
  `+openshift:enable:FeatureGate=AWSNestedVirtualization`; rename the type to
  `AWSNestedVirtualizationPolicy`; change the enum and constant values from the lowercase
  `enabled` / `disabled` copied from CAPA to `Enabled` / `Disabled`; and change the field from a
  pointer to a value type with `""` in the enum. See API Extensions for the rationale on the last
  three. The feature gate entry should follow `AWSDedicatedHosts` (`features.go`), which is the
  closest precedent — an AWS `providerSpec` field, Tech Preview and Dev Preview only — rather than
  `VSphereHostVMGroupZonal`, which has already graduated to `inDefault()`.
* **Feature gate enforcement happens in the webhook, not in CRD schema.** `providerSpec.value` is a
  `runtime.RawExtension` on the `Machine` and `MachineSet` CRDs, marked
  `x-kubernetes-preserve-unknown-fields`. Kubebuilder markers on `AWSMachineProviderConfig` therefore
  generate no CRD validation at all — this is why `make generate` produces no CRD manifest diff on
  openshift/api#3088. Gating and validation must be implemented in
  `openshift/machine-api-operator` in `pkg/webhooks/machine_webhook.go` and
  `pkg/webhooks/machineset_webhook.go`, which already hold a `featuregate.MutableFeatureGate` and use
  the `config.featureGates.Enabled(featuregate.Feature(apifeatures.FeatureGateX))` pattern (see the
  existing `VSphereHostVMGroupZonal` gate). The webhook rejects `cpuOptions.nestedVirtualization` when
  the gate is off, rejects values outside the enum (the `Enum` marker is inert here for the same
  reason, so this must be an explicit check mirroring the existing `AWSConfidentialComputePolicy`
  switch), and rejects `nestedVirtualization: Enabled` combined with
  `confidentialCompute: AMDEncryptedVirtualizationNestedPaging`.
* **The field is rejected on control plane machines for Tech Preview.** The Machine webhook rejects
  `cpuOptions.nestedVirtualization` on Machines carrying the control-plane role, and
  `cluster-control-plane-machine-set-operator` rejects it in a `ControlPlaneMachineSet` template.
  This is a deliberate restriction, not an oversight. `ControlPlaneMachineSet` performs a rolling
  replacement of control plane Machines whenever its `providerSpec` changes, with surge pinned to 1
  and create-before-delete ordering. A `providerSpec` that AWS rejects therefore does not cost etcd
  quorum — but the replacement Machine goes permanently `Failed`, the rollout wedges with four
  control plane Machines, and the `control-plane-machine-set` ClusterOperator degrades, **which
  blocks cluster upgrades by design**. `.spec.state` cannot be returned to `Inactive` once `Active`
  (CEL-enforced), so recovery means fixing the spec and deleting the failed Machine, switching the
  CPMS to `OnDelete`, or deleting the CPMS resource. With the recommended control plane
  MachineHealthCheck at `maxUnhealthy: 1`, the same misconfiguration becomes a delete-and-recreate
  loop on masters that interacts with the etcd deletion hooks. The blast radius is too large to
  carry through Tech Preview for a capability whose motivating workloads all run on workers.
  Revisiting this for compact and single-node clusters is deferred to GA.
* **openshift/machine-api-provider-aws** — extend `getCPUOptionsRequest` in
  `pkg/actuators/machine/instances.go`, which already maps `confidentialCompute` onto
  `ec2.CpuOptionsRequest.AmdSevSnp`, to map `nestedVirtualization` onto
  `CpuOptionsRequest.NestedVirtualization`. The function already returns `nil` for an empty struct,
  so an unset field continues to send no `CpuOptions` at all. The mapping itself is about ten lines
  — but see the blocking SDK dependency below, which is the real cost.
* **BLOCKING DEPENDENCY: `machine-api-provider-aws` must migrate to `aws-sdk-go-v2` first.**
  This is the single largest obstacle to delivering this enhancement, and it is a hard one — not a
  sequencing preference. It requires explicit agreement from the Machine API owners before any
  timeline in this document is meaningful.

  `machine-api-provider-aws` pins `github.com/aws/aws-sdk-go v1.55.8` (go.mod line 6) and has no v2
  dependency at all. The field this enhancement needs does not exist in v1, and cannot be added:

  * Every file in `service/ec2` at v1.55.8 — `api.go`, `customizations.go`, `doc.go`, `errors.go`,
    `service.go`, `waiters.go` — contains zero occurrences of `NestedVirtualization`, as does a
    code search across the whole `aws/aws-sdk-go` repository.
  * Both CPU options shapes are complete without it. `CpuOptions` (the response type, `api.go:70568`)
    and `CpuOptionsRequest` (the request type, `api.go:70620`) each carry exactly `AmdSevSnp`,
    `CoreCount`, and `ThreadsPerCore`.
  * v1.55.8 is both the latest tag and the final release, published 2025-07-31, and the
    `aws/aws-sdk-go` repository is **archived** on GitHub. It is read-only: there will be no further
    releases, so the 2026 EC2 API shape can never land in v1. **There is no `aws-sdk-go` version to
    bump to.**
  * The field exists only in `aws-sdk-go-v2`, on all four relevant shapes — request and response, for
    both `RunInstances` and `ModifyInstanceCpuOptions` — as
    `NestedVirtualization NestedVirtualizationSpecification`, with values `enabled` / `disabled`.

  Consequently the actuator work requires migrating the EC2 client — at minimum the `RunInstances`
  path — to aws-sdk-go-v2. That is cross-cutting: the client interface in `pkg/client`, the test
  stubs and mocks in `pkg/actuators/machine/stubs.go` and `instances_test.go`, and the error handling
  (`awserr.RequestFailure` at `instances.go:488` has no v2 equivalent and becomes `smithy.APIError` /
  `awshttp.ResponseError`).

  A full migration was attempted in
  [machine-api-provider-aws#180](https://github.com/openshift/machine-api-provider-aws/pull/180)
  (March 2026): context on all client methods, `aws/session` to `aws-sdk-go-v2/config`, value types
  from `service/*/types`, IMDSv2 via `feature/ec2/imds`, and per-service `BaseEndpoint` for custom
  endpoints. It was held by its author, went stale on rebase, and was closed un-merged in April 2026
  — no reviewer rejected it on principle. That history is the point: this migration has already
  stalled once, and reviving it needs an owner and a decision rather than an assumption. The
  non-vendor diff was 24 files and roughly 1,400 lines, over half of it tests.

  This needs its own tracking issue and owner, and is a prerequisite for Tech Preview — not a detail
  of this enhancement. The installer has already completed its own v1-to-v2 migration and vendors
  only aws-sdk-go-v2, so a reference implementation exists.
* **Mutability.** The field is mutable, with delayed effect. On a `MachineSet` template, editing it
  behaves like any other `providerSpec` change: existing Machines are untouched (MAPI MachineSets do
  not roll), and the new value applies to the next Machine created. On an individual `Machine`, the
  edit has no effect at all, because the actuator only passes `CpuOptions` at `RunInstances` time and
  this enhancement adds no stop/modify/start orchestration. No webhook immutability rule is added,
  matching the rest of `providerSpec` — but see Risks, because the Machine-level edit is a silent
  no-op.

### Risks and Mitigations

* **A user sets the field on an unsupported instance type or in an unsupported region.** AWS rejects
  `RunInstances`, the actuator returns `InvalidMachineConfiguration`, and the Machine goes to `Failed`
  with the AWS error on the Machine status. We deliberately do not maintain a hard-coded instance type
  allowlist in OpenShift, because AWS adds supported families over time and a stale list would reject
  valid configurations. This mirrors how `confidentialCompute` behaves today.
* **A MachineSet with an invalid combination blocks autoscaling.** Every Machine created from the
  MachineSet fails identically. This is the pre-existing failure mode for any bad `providerSpec`.
* **Nested virtualization has a performance cost and is not a security boundary.** This is an AWS and
  KVM property, not something OpenShift can mitigate; it will be covered in documentation.
* **Enum casing diverges from upstream CAPA.** CAPA uses lowercase `enabled` / `disabled`; this
  enhancement uses `Enabled` / `Disabled`. The MAPI-to-CAPI conversion in `cluster-capi-operator` must
  therefore map values rather than copy them. Mitigation: the conversion is a two-value switch
  covered by the conversion unit tests, and the alternative — a lowercase enum sitting next to a
  PascalCase `confidentialCompute` in the same struct — would be a permanent inconsistency in a
  GA OpenShift API.
* **The aws-sdk-go-v2 migration does not land, or lands late.** This is the largest schedule risk in
  the enhancement and it sits outside its control: the work is in a repo owned by another team, it
  has stalled once already, and nothing else here can proceed without it. There is no mitigation
  within this enhancement and no partial workaround — injecting an unmodelled parameter into the v1
  request, or running a second scoped v2 client alongside v1 purely for `RunInstances`, were both
  considered and are not acceptable (see Alternatives). The honest mitigation is to secure an owner
  and a target release for the migration before committing this enhancement to a release.
* **Until the conversion lands, a Machine using this field is unmigratable to Cluster API.** Per
  [converting-machine-api-to-cluster-api](converting-machine-api-to-cluster-api.md), when the sync
  controller cannot represent a Machine API field in Cluster API it sets the `Synchronized` condition
  to `False`; the migration controller then treats the resource as unmigratable and takes no action,
  and the documented remedy for an administrator is to contact support. Because
  `AWSNestedVirtualization` and `MachineAPIMigration` would both be Tech Preview gates, they are
  enabled together in exactly the clusters where this is first exercised.

  Two outcomes must be avoided. Silently dropping the field during conversion is the worse one: it
  produces a running Cluster API Machine on an instance with no `/dev/kvm` and no error anywhere.
  Setting `Synchronized=False` is the correct behaviour, but it strands the resource.

  Mitigation: the conversion in [OCPCLOUD-3315](https://issues.redhat.com/browse/OCPCLOUD-3315) is a
  hard prerequisite for GA rather than a parallel effort, and a Tech Preview e2e enables both gates
  together to prove the round trip. Note the conversion must handle PascalCase-to-lowercase in both
  directions; the value-type choice in API Extensions removes the pointer dereference that would
  otherwise also be required.

### Drawbacks

This adds another provider-specific knob to an API that is on a migration path to Cluster API, so the
field has to be carried through the MAPI-to-CAPI conversion in `cluster-capi-operator`. The
alternative — telling users to wait for CAPI — leaves the day-2 workaround in place for several
releases, which is worse.

## Alternatives (Not Implemented)

* **Keep the day-2 script.** Rejected for the reasons given in Motivation.
* **Automate the day-2 path inside the Machine API**, having the actuator call
  `ModifyInstanceCpuOptions` — stopping, modifying, and restarting an instance when
  `nestedVirtualization` changes. AWS permits this on a stopped instance, so the obstacle is not a
  platform limitation. Rejected: it makes a `providerSpec` edit silently
  destructive to a running workload, and contradicts the Machine API's model, where changing a
  Machine's shape means replacing the Machine.
* **Deliver this only in Cluster API / CAPA.** This deserves more than a scheduling answer, because
  [converting-machine-api-to-cluster-api](converting-machine-api-to-cluster-api.md) lists "port any
  feature from Cluster API into Machine API" as an explicit Non-Goal, and that is literally what this
  enhancement does. Rejected nonetheless, for two reasons. First, `nestedVirtualization` is not a
  Cluster API feature being back-ported; it is an AWS platform capability that both stacks expose
  independently, in the same struct both already have. Second, the same migration enhancement states
  that the Machine API will exist in OpenShift 4 indefinitely and that Machine API remains
  authoritative for self-managed AWS today, so declining to add the field does not accelerate
  migration — it just leaves the destructive day-2 workaround in place for the clusters that are not
  migrating yet. The Machine API approver should confirm this exception explicitly.
* **Add a separate top-level `nestedVirtualization` field on `AWSMachineProviderConfig` instead of
  nesting it under `cpuOptions`.** Rejected: AWS models it as part of `CpuOptions`, and CAPA and the
  existing `confidentialCompute` field already use that grouping.
* **Infer nested virtualization from the instance type.** Rejected: nested virtualization is opt-in at
  AWS, carries a performance cost, and silently enabling it on every C8i/M8i/R8i would be a surprising
  behaviour change.
* **Avoid the aws-sdk-go-v2 migration.** Two routes were considered and both rejected. *Inject the
  unmodelled parameter into the v1 request* — EC2 uses a form-encoded query protocol, so a custom
  v1 `Build` handler could append `CpuOptions.NestedVirtualization` to the request body. It would
  work, but it is invisible to the type system, untestable through the normal client mocks, and
  silently breaks if AWS changes the shape. *Run a scoped v2 client alongside v1 for `RunInstances`
  only* — the two module paths can coexist, but `machine-api-provider-aws` wires custom per-service
  endpoints, static credentials, IMDSv2, and region validation through its v1 session, so this means
  maintaining two credential paths and two error-handling idioms indefinitely to avoid a migration
  that is needed anyway. Neither is worth doing to dodge a dependency that the repository has to
  take on regardless, given v1 is archived and unsupported.

## Open Questions [optional]

1. **Conflict validation placement.** The CEL `XValidation` rule on `CPUOptions` in openshift/api#3088
   is inert for `providerSpec` (see Implementation Details). Should it be dropped from openshift/api to
   avoid implying enforcement that does not exist, kept so that CAPI consumers of the same struct
   inherit it, or both kept and duplicated in the webhook? The proposal above assumes the last option.
2. **Lifting the control plane restriction at GA.** Tech Preview rejects the field on control plane
   Machines and `ControlPlaneMachineSet` (see Implementation Details). Compact and single-node
   clusters that run VM workloads on the control plane are a plausible future use case. Lifting the
   restriction needs the nested-virt CPU overhead quantified against the SNO resource envelope, and
   agreement with `cluster-control-plane-machine-set-operator` owners on how a rejected
   `RunInstances` during a control plane rollout should surface. Deferred to GA, not to
   implementation.

## Test Plan

* **Unit tests** in `machine-api-provider-aws` covering `getCPUOptionsRequest`: field unset produces no
  `CpuOptions`, `Enabled` and `Disabled` produce the corresponding `CpuOptionsRequest`, and the field
  composes correctly with `confidentialCompute`.
* **Unit tests** in `machine-api-operator` webhooks covering: rejection when the gate is off,
  acceptance when the gate is on, and rejection of the SEV-SNP plus nested virtualization combination,
  for both `Machine` and `MachineSet`.
* **At least five e2e tests**, meeting the minimum in `dev-guide/feature-zero-to-hero.md`. All carry
  the `[OCPFeatureGate:AWSNestedVirtualization]` label so the feature-promotion analyzer picks them
  up, plus the `[Jira:"Cloud Compute / Machine API"]` component label. Creating a MachineSet and
  waiting for a Node makes these `[Slow]`, and asserting on `/dev/kvm` requires a privileged pod or
  `oc debug node`, so the suite registration and job configuration need to be agreed with the
  Machine API and TRT owners before the tests are written.
  1. Happy path: MachineSet with a supported instance type and `nestedVirtualization: Enabled`
     produces a Ready Node exposing `/dev/kvm`.
  2. `nestedVirtualization: Disabled` and unset both produce a Node without `/dev/kvm`.
  3. Unsupported instance type produces a `Failed` Machine with `errorReason=InvalidConfiguration`
     and the AWS error in `errorMessage` — asserting the negative path is legible, not just that it
     fails.
  4. Gate-off rejection: with the gate disabled, MachineSet creation carrying the field is rejected
     by the webhook, and the rejection message names the field.
  5. Replacement durability: deleting a Machine from a nested-virt MachineSet produces a replacement
     that still has `/dev/kvm` — this is the property the day-2 workaround lacks.
  6. Control plane rejection: a `ControlPlaneMachineSet` carrying the field is rejected.
* **A daily periodic** on AWS for both the `TechPreviewNoUpgrade` and `Default` job variants. One
  daily run satisfies the ≥7 runs/week minimum; the `Default` variant skips the tests until the gate
  promotes but ensures coverage continues afterwards.
* The e2e suite does **not** assert that a nested VM actually boots. Proving that a Kata or CNV
  workload runs on these nodes is owned by OSC and CNV and is tracked as a GA criterion below, not
  as a Machine API test.

## Graduation Criteria

### Dev Preview -> Tech Preview

This feature enters directly at Tech Preview. The `AWSNestedVirtualization` gate is enabled in both
`DevPreviewNoUpgrade` and `TechPreviewNoUpgrade` from the first release that carries it.

* **Prerequisite:** the `aws-sdk-go-v2` migration of the `RunInstances` path in
  `machine-api-provider-aws` is merged (see Implementation Details). Nothing else in this list can
  start until it lands.
* API field, feature gate, webhook validation, and actuator plumbing all merged.
* Unit tests and the e2e job described above passing.
* Sufficient documentation for an administrator to configure a MachineSet.

### Tech Preview -> GA

The bar is the one in `dev-guide/feature-zero-to-hero.md`, which this feature cannot meet unmodified
because it is AWS-only. Stating the gap and the exception path up front is deliberate — it is the
most likely reason GA slips.

* **Testing requirements met on AWS:** at least 5 tests, run at least 7 times per week, at least 14
  times on each covered platform, passing at least 95% of the time, in both `TechPreviewNoUpgrade`
  and `Default` variants, all in place no less than 14 days before branch cut for the target release.
* **An SBAR exception filed and approved for the non-AWS platforms.** Promotion to `Default`
  normally requires coverage across all eight platform/topology/architecture/network combinations
  (AWS HA, AWS Single, Azure, GCP, vSphere, and Baremetal IPv4/IPv6/Dual). `CpuOptions` is an EC2
  concept and does not exist on the other providers, so this is the "explicitly not supported on
  that platform" case, which is the most straightforward exception to obtain — but it must still be
  filed, and it carries an obligation to report weekly regression probability until equivalent
  coverage exists. AWS Single is covered by the control plane decision below, not by a worker test.
* Open Question 1 is resolved and reflected in the merged API.
* End-user documentation published, including the supported instance types; the fact that the setting
  is applied at instance launch, so enabling it on existing nodes means replacing those Machines; that
  a downgrade silently strips the field and it must be re-applied afterwards; and the mutual exclusion
  with AMD SEV-SNP.
* **MAPI-to-CAPI conversion ([OCPCLOUD-3315](https://issues.redhat.com/browse/OCPCLOUD-3315)) is
  complete.** This is not waivable: shipping a GA field that renders a Machine unmigratable, or that
  is silently dropped on conversion, is not an acceptable end state (see Risks).
* Installer surfacing ([CORS-4541](https://issues.redhat.com/browse/CORS-4541)) is complete or
  explicitly agreed as not blocking. Unlike the conversion, this one genuinely is separable — the
  field is usable via MachineSets without it.
* OSC or CNV has validated a real nested workload on a node provisioned this way, closing the gap
  between "the Node has `/dev/kvm`" and the user stories in Motivation.
* The control plane restriction is either lifted with evidence or documented as permanent
  (Open Question 2).
* The `AWSNestedVirtualization` gate is promoted to `inDefault()`.

### Removing a deprecated feature

Not applicable. No existing feature is deprecated or removed.

## Upgrade / Downgrade Strategy

The field is optional and additive, so upgrades require no action. While the feature is Tech Preview
the gate is only available in `TechPreviewNoUpgrade`, which is itself not upgradeable, so there is no
Tech Preview upgrade path to consider.

Downgrade is not supported for clusters that used the feature, and is **destructive to the
configuration**, not merely to the capability.

Decoding is not the problem: `machine-api-provider-aws` uses non-strict `json.Unmarshal` to decode
`providerSpec`, and the Machine API webhook's unknown-field check emits only an admission warning, so
an older build does not reject or fail a Machine carrying `nestedVirtualization`. The problem is the
**mutating** webhook, which round-trips the whole `providerSpec` on every admission: it unmarshals
into the older `AWSMachineProviderConfig`, re-marshals, and overwrites `providerSpec.value.Raw`. An
older build has no `nestedVirtualization` field, so the first create or update admission after
downgrade silently **strips the field out of etcd**. The same applies to MachineSets via the
MachineSet defaulter.

The practical consequences:

* Already-running instances keep nested virtualization, because it is a property of the launched EC2
  instance and nothing reconciles it away.
* The field survives in etcd only until the next admission pass touches the object.
* Re-upgrading does **not** restore it. Administrators must re-apply the field to every affected
  MachineSet and then replace the Machines.
* No error or event is produced at any point in this sequence.

## Version Skew Strategy

The only skew is between the Machine API webhooks (which gate the field) and
`machine-api-provider-aws` (which consumes it). Both ship in the same payload and the Machine API
operator rolls them out together, so the window is a single rollout. During it, a Machine admitted
with `nestedVirtualization` set may be created by an older actuator that ignores the field; the
instance comes up without nested virtualization and the Machine is not marked failed, so it must be
replaced. No kubelet, Node, or AWS API skew is involved.

## Operational Aspects of API Extensions

The change adds a handful of field checks to the two existing Machine API admission webhooks and to
the `ControlPlaneMachineSet` webhook — no new webhook, watch, or API call in the reconcile path — so
the effect on API server latency and reconciliation throughput is negligible, and the failure modes
are the existing Machine API webhook ones. No new SLI is proposed: a misconfigured `providerSpec`
surfaces as a `Failed` Machine and increments the existing failed-instance-create counter.

## Support Procedures

**A Machine stays in `Failed` with an AWS error about nested virtualization or CPU options.** The
instance type, region, or AMI does not support it. Check `machine.status.errorMessage`, then compare
the `providerSpec` instance type against the
[AWS supported instance list](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/amazon-ec2-nested-virtualization.html).

**A MachineSet is rejected with an error about `cpuOptions.nestedVirtualization`.** Either the
`AWSNestedVirtualization` gate is off (`oc get featuregate cluster -o yaml`), or `Enabled` was
combined with `confidentialCompute: AMDEncryptedVirtualizationNestedPaging`, which AWS disallows.

**The Node comes up but `/dev/kvm` is missing.** The instance predates the setting or was launched by
a skewed actuator (see Version Skew Strategy). Confirm with `aws ec2 describe-instances
--instance-ids <id> --query 'Reservations[].Instances[].CpuOptions'` and replace the Machine.

The existing `machine-api` must-gather already collects `Machine` and `MachineSet` objects including
`providerSpec`, so no new data collection is needed.

## Infrastructure Needed [optional]

The Tech Preview e2e job needs an AWS account and region with C8i, M8i, or R8i capacity available to
CI. No new repositories are required; all changes land in `openshift/api`,
`openshift/machine-api-operator`, and `openshift/machine-api-provider-aws`.
