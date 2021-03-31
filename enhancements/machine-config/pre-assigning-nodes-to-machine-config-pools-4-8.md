---
title: pre-assigning-nodes-to-machine-config-pools-4-8
authors:
  - "@beekhof"
reviewers:
  - "@slintes" 
  - "@markmc"
  - "@JoelSpeed"
  - "@alexcrawford"
  - "@kikisdeliveryservice"
approvers:
  - "@markmc"
  - "@JoelSpeed"
  - "@alexcrawford"
  - "@kikisdeliveryservice"
creation-date: 2021-03-31
last-updated: 2021-03-31
status: provisional
see-also:
  - "/openshift/enhancements/pull/717"
replaces:
  - 
superseded-by:
  - 
---

# Short-term approach for Pre-assigning Nodes to Machine Config Pools

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

In telco bare-metal environments, there is a need to have Nodes be provisioned
with a known Machine Config Pool.

The traditional flow is for Nodes to come up as workers and be moved to the
desired pool after provisioning, however this consumes a significant portion of
the maintenance window in bare-metal environments.

## Motivation

In telco environments, there is a fixed window (typically 4 hours, half of which
is reserved for rollback) during which new hardware needs to be installed and
provisioned.

In the case of remote worker nodes, they will typically need to be a part of a
specific (non-default) MachineConfigPool, and are provisioned with the correct
pool based on the Ingnition file contents.

However, the MCO/MCD normally manages MCP assignment and does so based on Node
Labels.  This creates a race to add the necessary labels before the MCO/MCD
moves the Node back to the default config pool.

On bare metal, the cost of loosing this race is significant, and as a result we
could spend half of the 2 hour window just rebooting.

### Goals

- Provide a CR that defines a set of labels that should be applied based on a Node's name
- Provide the ability to apply these labels *before* the Node object is created
- Provide the ability to have changes to the CRs be reflected on existing Nodes
- Meet critical telco use cases by shipping a solution in 4.8

### Non-Goals

- Establish a long-term direction for addressing this within the product.  See
  [enhancement #717](https://github.com/openshift/enhancements/pull/717) for
  discussion around an in-product approach

## Proposal

Two new CRs, a controller to watch each, and a webhook.

The first CR allows the admin to define patterns and a list of labels that
should be applied to any Node with a matching name.

```go
type LabelsSpec struct {
	// NodeNamePatterns defines a list of node name regex patterns for which the given labels should be set.
	// String start and end anchors (^/$) will be added automatically
	NodeNamePatterns []string `json:"nodeNamePatterns"`

	// Label defines the labels which should be set if one of the node name patterns matches
	// Format of label must be domain/name=value
	Labels map[string]string `json:"labels"`
}
```

This CR is used by a MutatingAdmissionWebhook when a new Node is created to
ensure the labels necessary for MCP placement are present from the moment the
Node object exists.  Ensuring that the Ignition file and MCO are in agreement as
to the correct pool assignment.

A controller also watches these CRs to ensure that any additions or
modifications are applied to existing Nodes, providing a consistent name-based
label management experience.

Deletions however, require special handling.  Deleting any label that is not
part of a `Labels` CR would be dangerous and likely to break the cluster -
because it cannot account for labels applied manually by the admin, or by other
components.

Therefore we define an `OwnedLabels` CRD which tells us which label domains the
operator owns exclusively.  When a label in this domain is no longer present in
a Labels CR, it is removed.

```go
// OwnedLabelsSpec defines the desired state of OwnedLabels
type OwnedLabelsSpec struct {
	// Domain defines the label domain which is owned by this operator
	// If a node label
	// - matches this domain AND
	// - matches the namePattern if given AND
	// - no label rule matches
	// then the label will be removed
	Domain *string `json:"domain,omitempty"`

	// NamePattern defines the label name pattern which is owned by this operator
	// If a node label
	// - matches this name pattern AND
	// - matches the domain if given AND
	// - no label rule matches
	// then the label will be removed
	// String start and end anchors (^/$) will be added automatically
	NamePattern *string `json:"namePattern,omitempty"`
}
```

### User Stories

1. As an admin, I want Nodes to have a specific set of labels based on their
   https://www.clli.com driven name, so that I can target workloads to machines
   based on hardware profile, location, etc.

1. As an admin, I want Nodes to be created with a set of labels consistent with
   the selection criteria of the MCP that it was provisioned with, so that it does
   not flap between pools, consuming valuable time rebooting.

### Implementation Details/Notes/Constraints [optional]

- Implementation: https://github.com/openshift-kni/node-label-operator
- Demo: https://drive.google.com/file/d/1IGaDDbgsm8CoTPyTOiD-agRcTy-iroQ-/view

### Risks and Mitigations

We could find a better way to achieve this using some combination of core
components (MAO/MCO), making this implementation redundant.

RBAC rules will need to ensure that only priviledged personas are able to create
Labels and OwnedLabels CRs.  Otherwise it could be a vector for DoS or
privelidge escalation attacks.

## Design Details

Nothing additional

### Test Plan

**Note:** *Section not required until targeted at a release.*

Consider the following in developing a test plan for this enhancement:
- Will there be e2e and integration tests, in addition to unit tests?
- How will it be tested in isolation vs with other components?
- What additional testing is necessary to support managed OpenShift service-based offerings?

No need to outline all of the test cases, just the general strategy. Anything
that would count as tricky in the implementation and anything particularly
challenging to test should be called out.

All code is expected to have adequate tests (eventually with coverage
expectations).

### Graduation Criteria

**Note:** *Section not required until targeted at a release.*

Define graduation milestones.

These may be defined in terms of API maturity, or as something else. Initial proposal
should keep this high-level with a focus on what signals will be looked at to
determine graduation.

Consider the following in developing the graduation criteria for this
enhancement:

- Maturity levels
  - [`alpha`, `beta`, `stable` in upstream Kubernetes][maturity-levels]
  - `Dev Preview`, `Tech Preview`, `GA` in OpenShift
- [Deprecation policy][deprecation-policy]

Clearly define what graduation means by either linking to the [API doc definition](https://kubernetes.io/docs/concepts/overview/kubernetes-api/#api-versioning),
or by redefining what graduation means.

In general, we try to use the same stages (alpha, beta, GA), regardless how the functionality is accessed.

[maturity-levels]: https://git.k8s.io/community/contributors/devel/sig-architecture/api_changes.md#alpha-beta-and-stable-versions
[deprecation-policy]: https://kubernetes.io/docs/reference/using-api/deprecation-policy/

**Examples**: These are generalized examples to consider, in addition
to the aforementioned [maturity levels][maturity-levels].

#### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers
- Enumerate service level indicators (SLIs), expose SLIs as metrics
- Write symptoms-based alerts for the component(s)

#### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default
- Backhaul SLI telemetry
- Document SLOs for the component
- Conduct load testing

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

#### Removing a deprecated feature

- Announce deprecation and support policy of the existing feature
- Deprecate the feature

### Upgrade / Downgrade Strategy

NA

### Version Skew Strategy

New component

## Implementation History

- Initial version: 2021-03-31

## Drawbacks

The idea is to find the best form of an argument why this enhancement should _not_ be implemented.

## Alternatives

1. Applying labels from Machines and/or MachineSets happens too late in the
   process and does not close the possibility for race conditions, nor does it
   apply to adding UPI workers.

2. Node Feature Discovery can (as a result of our work) apply labels based on
   Node names, but it also happens too late in the process and does not close
   the possibility for race conditions

3. Modifying kubelet.service as part of the initial Ignition configuration
   works, but is risky if that file ever needs to change, and considered a hack.

4. Modifying some combination of MAO/MCO/MCD is considered unjustified by some
   but may be a better long term approach.  See [enhancement #717](https://github.com/openshift/enhancements/pull/717).

## Infrastructure Needed [optional]

- Bugzilla Component
