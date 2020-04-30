---
title: operator-condition-links
authors:
  - "@wking"
reviewers:
  - "@abhinavdahiya"
  - "@LalatenduMohanty"
approvers:
  - "@deads2k"
  - "@sdodson"
creation-date: 2020-04-24
last-updated: 2020-04-30
status: implementable
---

# Operator Condition Links

When an operator condition is in a surprising state (`Available=False`, `Degraded=True`, etc.), it's not always clear from the short reason and message what the impact or recommended mitigations are.
By including structured links in the condition, we make it easy for all condition consumers to point to external documentation, where there is more space for detailed discussion.

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [x] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Open Questions

Are there restrictions on URI targets?
Do they all need to point at either docs.openshift.com or Knowledgebase articles?
Is it ok to also link in-cluster web console pages (e.g. "fix this over here...")?
GitHub pages?
kubernetes.io/docs/?
Other external docs?
These URIs are baked into release images and need to continue to resolve to their intended target for the supported life of that release, so when there is concern about target stability, we may want to point at an off-cluster redirection service.

Do we require these doc links?
Some messages may be sufficient without links, in which case creating a doc-link may be busywork.
On the other hand, even relatively straightforward stuff like [`VersionNotFound`][version-not-found] could benefit from a few lines of context, which is more than we want to bake into the message string itself.
Deciding this might benefit from a survey of conditions collected in Insights.

## Summary

Add a `links` property to [`ClusterOperatorStatusCondition`][api-condition].

## Motivation

For [the `DefaultSecurityContextConstraints_Mutated` issue][rhbz-1821905], we had user-facing documentation landing in [the bug][rhbz-1821905c22] and [in a Knowledgebase article][modified-SCCs-kb].
However, there was no in-cluster way to point cluster administrators at that documentation.
By adding links to the condition structure, all in-cluster components that are raising awareness about the surprising condition will be able to link cluster administrators to the detailed "now what?" documentation.

### Goals

All surprising conditions will declare at least one documentation URI, or be able to defend their existing message as being sufficient to describe the condition's impact and mitigation ideas.

### Non-Goals

We also alert on surprising and dangerous conditions.
Alerts do not allow for structured links, and also rely on message annotations to describe the condition's impact and mitigation ideas.
But alerts are also an established upstream idea, and altering them would be a different process than the OpenShift enhancement for altering the operator status API.

## Proposal

Define a new `Link` type:

```go
// Link represents a minimal hyperlink, which would map to HTML like
// <a href={uri}>{name}</a>.
type Link struct {
	// Name is a short phrase describing the link.
	// +required
	Name string `json:"name"`

	// URI is the address of the link.
	// +required
	URI URL `json:"uri"`
}
```

Add a `Links` property to [`ClusterOperatorStatusCondition`][api-condition]:

```go

// Links provides hyperlinks to additional information about this
// condition.  Links may describe the impact on the cluster or provide
// steps to mitigate or resolve the issue.
// +optional
Links []Link `json:"links,omitempty"
```

### User Stories

#### OperatorSource/CatalogSourceConfig deprecation

This enhancement would provide structured support for existing conditions like [the marketplace operator's `Upgradeable=False`][rhbz-1827775c0]:

> The cluster has custom OperatorSource/CatalogSourceConfig, which are deprecated in future versions.  Please visit this link for further details: https://docs.openshift.com/container-platform/4.4/release_notes/ocp-4-4-release-notes.html#ocp-4-4-marketplace-apis-deprecated

Which would become:

```json
{
  "lastTransitionTime": "2000-01-01T00:00:00Z",
  "type": "Upgradeable",
  "status": "False",
  "reason": "DeprecatedAPIsInUse",
  "message": "The cluster has custom OperatorSource/CatalogSourceConfig, which are deprecated in future versions.",
  "links": [
    {
      "name": "4.4 release notes",
      "uri": "https://docs.openshift.com/container-platform/4.4/release_notes/ocp-4-4-release-notes.html#ocp-4-4-marketplace-apis-deprecated",
    }
  ]
}
```

#### Available updates conditions

The cluster-version operator documents a number of conditions around update retrieval.
Failing conditions could link to those documentation, with entries like:

```json
{
  "lastTransitionTime": "2000-01-01T00:00:00Z",
  "type": "RetrievedUpdates",
  "status": "False",
  "reason": "NoChannel",
  "message": "The update channel has not been configured.",
  "links": [
    {
      "name": "",
      "uri": "https://github.com/openshift/cluster-version-operator/blob/release-4.5/docs/user/status.md#nochannel",
    }
  ]
}
```

### Risks and Mitigations

What are the risks of this proposal and how do we mitigate. Think broadly. For
example, consider both security and how this will impact the larger OKD
ecosystem.

How will security be reviewed and by whom? How will UX be reviewed and by whom?

Consider including folks that also work outside your immediate sub-project.

## Design Details

### Test Plan

FIXME: If we have [restrictions on link targets](#open-questions), we should extend [the *Monitor cluster while tests execute* logic][test-monitor] to fail tests if a condition links to an unacceptable target.

FIXME: If we require surprising conditions to have doc links, we should extend [the *Monitor cluster while tests execute* logic][test-monitor] to fail tests if a surprising condition lacks links.

### Graduation Criteria

This will start as GA.

### Upgrade / Downgrade Strategy

On upgrade, operators will gain access to the new property and can begin writing conditions that use it.
On downgrade, operators will lose access to the property and will have to return to inlining URIs in their message text.

Consumers that expect the current condition schema may miss these links.
For example, if [the OperatorSource/CatalogSourceConfig deprecation](#operatorsource-catalogsourceconfig-deprecation) moves the documentation link into the structured field, legacy consumers would no longer have access to the link.
This is mitigated by there probably not being that many condition consumers in third-party code, and the links being supplemental to the message which legacy consumers will still have access to.

### Version Skew Strategy

We would roll this out over two minors:

1. 4.y: Add the new property to the CRD.
2. 4.(y+1): Begin teaching operators to set the new property.

If they both happened in the same minor release, the CRD would be downgraded to drop the property before the operators had been downgraded to stop setting it.

## Implementation History

1. Enhancement proposed.

## Drawbacks

We definitely want to improve linking to impact and mitigation documents, via this proposed change or an alternative.
There is no downside.

## Alternatives

The [existing condition structure][api-condition] could be tweaked to claim the message as a Markdown string, or some such, that allowed for structured linking.
This simplifies the condition schema and avoids punting on "how should I format the separate links and message for my users?".
But it also means that condition-writing operators and condition-consumers need to agree on a particular Markdown flavor, and consumers interested in rendering in HTML or other structured format would need to vendor markdown renderers.

Rolling it out over one minor and setting [`x-kubernetes-preserve-unknown-fields: true`][crd-schema] to avoid issues on downgrade.
This would let us get useful information into the conditions more quickly, but would mean that during [the version-skew phase](#version-skew-strategy), admins would have the link-less message strings without the supplemental link property.
Preserving links during skewed downgrades seems more important than a quick roll-out.

[api-condition]: https://github.com/openshift/api/blob/0422dc17083e9e8df18d029f3f34322e96e9c326/config/v1/types_cluster_operator.go#L109-L136
[crd-schema]: https://kubernetes.io/docs/tasks/access-kubernetes-api/custom-resources/custom-resource-definitions/#specifying-a-structural-schema
[modified-SCCs-kb]: https://access.redhat.com/solutions/4972291
[rhbz-1821905]: https://bugzilla.redhat.com/show_bug.cgi?id=1821905
[rhbz-1821905c22]: https://bugzilla.redhat.com/show_bug.cgi?id=1821905#c22
[rhbz-1827775c0]: https://bugzilla.redhat.com/show_bug.cgi?id=1827775#c0
[test-monitor]: https://github.com/openshift/origin/blob/5c167724f4a2c63064acf19c90e0445ad384f5d8/pkg/test/ginkgo/cmd_runsuite.go#L287
[version-not-found]: https://github.com/openshift/cluster-version-operator/blob/release-4.5/docs/user/status.md#VersionNotFound
