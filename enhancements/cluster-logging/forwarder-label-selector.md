---
title: forwarder-label-selector
authors:
  - "@alanconway"
reviewers:
  - TBD
approvers:
  - TBD
creation-date: 2020-08-25
status: provisional
---

# Forwarder Label Selector

## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [X] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Add an input selector to the ClusterLogForwarder (CLF) to collect application
logs only from pods identified by labels.

Kubernetes has two ways to identify pods: namespaces and labels.  The CLF
already has an input selector for namespaces, this enhancement will add a selector
for labels.

See also:
* [Kubernetes Labels and Selectors](https://kubernetes.io/docs/concepts/overview/working-with-objects/labels)

## Motivation

Users want to forward logs from specific applications. The forwrader can already
forward logs from selected namespaces, but many kubernetes applications use
_labels_ to identify logical application components, so we need to also allow
logs to be selected by label.

### Goals

* Support [equality and set based selectors](https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#label-selectors)
* Use the same field names as the standard k8s [LabelSelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.18/#labelselector-v1-meta) type.

## Proposal

Extend the `clusterlogforwarder.inputs.application` by adding a selector field:

```go
import  metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

type Application struct {
    // Namespaces selects logs from pods in one of the listend namespaces.
    //
    // +optional
    Namespaces []string `json:"namespaces,omitempty"`

+   // Selector selects logs from all pods with matching labels.
+   //
+   // +optional
+   Selector *metav1.LabelSelector `json:"selector,omitempty"`
}
```

The [Go definition of `LabelSelector`](https://godoc.org/k8s.io/apimachinery/pkg/apis/meta/v1#LabelSelector) is included here for reference:

```go
// A label selector is a label query over a set of resources. The result of matchLabels and
// matchExpressions are ANDed. An empty label selector matches all objects. A null
// label selector matches no objects.
type LabelSelector struct {
	// matchLabels is a map of {key,value} pairs. A single {key,value} in the matchLabels
	// map is equivalent to an element of matchExpressions, whose key field is "key", the
	// operator is "In", and the values array contains only "value". The requirements are ANDed.
	// +optional
	MatchLabels map[string]string `json:"matchLabels,omitempty" protobuf:"bytes,1,rep,name=matchLabels"`
	// matchExpressions is a list of label selector requirements. The requirements are ANDed.
	// +optional
	MatchExpressions []LabelSelectorRequirement `json:"matchExpressions,omitempty" protobuf:"bytes,2,rep,name=matchExpressions"`
}
```
### Implementation phases

May be implemented in 2 phases:

1. Implement only `MatchLabels`, intsead of `meta.LabelSelector` use this struct:\
   `type LabelSelector struct { MatchLabels map[string]string }`

2. Add implementation of Matchexpressions, using the original `meta.LabelSelector`

### User Stories

#### Select logs using simple equality-based selector

For exmaple, forward logs with labels `environment=product` and `app=nginx`:


```yaml
apiVersion: "logging.openshift.io/v1"
kind: "ClusterLogForwarder"
metadata:
  name: "instance"
spec:
  pipelines:
    - inputRefs: [ myLogs ]
      outputRefs: [ default  ]
  inputs:
    - name: myLogs
      application:
        selector:
          matchLabels:
            environment: production
            app: nginx
```

#### Select logs using both equality-based and set-based selectors

For example, forward logs with label `component=redis` where label `tier` has
value `cache` or `proxy` and label `environment` is not `dev`


```yaml
ppapiVersion: "logging.openshift.io/v1"
kind: "ClusterLogForwarder"
metadata:
  name: "instance"
spec:
  pipelines:
    - inputRefs: [ myLogs ]
      outputRefs: [ default  ]
  inputs:
    - name: myLogs
      application:
        selector:
          matchLabels:
            component: redis
          matchExpressions:
            - {key: tier, operator: In, values: [cache, proxy]}
            - {key: environment, operator: NotIn, values: [dev]}
```

#### Combine namespace and label selectors

For example: forward logs from namespaces `app1` or `app2` with label
`environment=production`


```yaml
apiVersion: "logging.openshift.io/v1"
kind: "ClusterLogForwarder"
metadata:
  name: "instance"
spec:
  pipelines:
    - inputRefs: [ myLogs ]
      outputRefs: [ default  ]
  inputs:
    - name: myLogs
      application:
        selector:
          matchLabels:
            environment: production
        namespaces:
        - app1
        - app2
```

### Implementation Details

The implementation will use a fluentd plug-in to filter based on namespace and labels.
The existing namespace selector logic will be replaced to also use this plugin.
A single filter point is simpler and should perform better.

This proposal can be implemented in two parts:

1. Implement only the `MatchLabels` equality based selector.
   Focus on matching with simple logic.
2. Add `MatchExpressions` support with the more complex parsing and matching logic.

### Risks and Mitigation

* Performance of new filter plugin must be acceptable.
* This feature restricts the data exposed by the forwarder, so no new security concerns.

## Design Details

### Open Questions
None

### Test Plan
TODO

## Implementation History
TODO

## Drawbacks
None.

## Alternatives

Forward all logs and filter in an external agent. We forward log events with
labels attached an external agent could do this filtering.
