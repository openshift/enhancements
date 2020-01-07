---
title: Cluster-Resource-Overrides-Enablement
authors:
  - "@deads2k"
reviewers:
  - "@sttts"
  - "@derekwaynecarr"
approvers:
  - "@derekwaynecarr"
creation-date: 2019-09-11
last-updated: 2019-09-11
status: provisional
see-also:
replaces:
superseded-by:
---

# ClusterResourceOverrides Enablement

The `autoscaling.openshift.io/ClusterResourceOverride` cannot be enabled in 4.x.  The plugin already exists, this design 
is about how we make it possible for a customer to enable the feature.

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift/docs]

## Summary

The `autoscaling.openshift.io/ClusterResourceOverride` admission plugin is an uncommonly used admission plugin with configuration
values.  Because it is uncommonly used, it doesn't fit well with our targeted configuration which aims to avoid adding
lots of intricately documented knobs.  Instead of wiring the admission plugin via a kube-apiserver operator, we can create
a mutating admission webhook based on the [generic-admission-server](https://github.com/openshift/generic-admission-server)
and install it via OLM.

## Motivation

The `autoscaling.openshift.io/ClusterResourceOverride` admission plugin is used for over-commit, let's stipulate that it is
important enough to enable.  The kube-apiserver is designed to be extended using mutating admission webhooks, we have the
technology to easily build one, and we have the ability to create a simple operator to manage it.  We want to enable the 
feature using a pattern that we can extend to other admission plugins that can scale beyond the small team that maintains
the kube-apiserver.

### Goals

1. Enable the `autoscaling.openshift.io/ClusterResourceOverride` admission plugin that is used for overcommit.
2. Use existing extension points, libraries, and installation mechanisms in the manner we would recommend to 
 external teams.
3. Have a fairly straightforward way to install and enable this admission plugin.
4. Rebootstrapping must be possible.

### Non-Goals

1. Revisit how `autoscaling.openshift.io/ClusterResourceOverride` works.  We're lifting it as-is.
2. Couple a slow moving admission plugin to a fast moving kube-apiserver.

### Open Questions

1. Do we need to protect openshift resources from being overcommitted?  Perhaps the cluster-admin's intent is exactly that.
2. We cannot uniformly apply protection just to our payload resources, how do we position this?
 External teams may be surprised that their resource requirements are not respect, but ultimately the cluster-admin is in
 control of his cluster.  This is what running self-hosted means.
3. How are OLM operators tested against OpenShift levels?
4. How do we build and distribute this OLM operator using OpenShift CI?
5. How do we describe version skew limitations to OLM so our operator gets uninstalled *before* an illegal downgrade or upgrade?
 This is a concrete case of the API we want to use isn't available before 1.16 and after 1.18, the previous API could be gone.

## Proposal

1. Create a mutating admission webhook server that provides `autoscaling.openshift.io/ClusterResourceOverride`.
2. Create an operator that can install, maintain, and configure this mutating admission webhook.
3. Ensure that we consistently label all prereq namespaces (we attempted runlevel before so this may work), to be sure
 that we re-bootstrap.
4. Expose the new operator via OLM and integrate our docs that way.

### User Stories [optional]

#### Story 1

#### Story 2

### Implementation Details/Notes/Constraints [optional]

We *must* be able to re-bootstrap the cluster.  This means that a cluster with this admission plugin created must be able
to be completely shut down and subsequently restarted:

* The `Admission Webhook` server will be located on the same node as the `kube-apiserver`.
* The `Admission Webhook` server should be reachable to the `kube-apiserver` over `localhost` of `hostnetwork`.
* Communication between `kube-apiserver` and the `Admission Webhook` server must be secure using `x509`. 
* The `Admission Webhook` server should only allow connection over `localhost`, thus it would disallow external connection(s).

This is how we are going to deploy the webhook server.
* Use `DaemonSet` to deploy the pods of the `Admission Webhook` server. 
* Use `hostnetwork` for `PodSpec`, set `hostnetwork: true`.
* Select a host port that is available.
* Apply appropriate `nodeSelector` and `tolerations` to the `PodSpec` so that the `Pods` are scheduled on to master node(s). This way `kube-apiserver` can access the webhook server using `localhost`.
* The `Admission Webhook` generates `localhost` serving certs. 
* The Admission Webhook binds to `127.0.0.1` to disable external connection.
* `kube-apiserver` uses `https://localhost` to access the admission webhook server. `clientConfig.URL` of the `MutatingWebHookConfiguration` or `ValidatingWebhookConfiguration` should be set to `https://localhost:{host port}/apis/{api path}`.

The following sets up `ClusterResourceOverride` admission webhook server over `localhost` of `hostnetwork`. The admission webhook serves the following API.
```yaml
/apis/admission.autoscaling.openshift.io/v1/clusterresourceoverrides

Group: admission.autoscaling.openshift.io
Version: v1
Resource: clusterresourceoverrides
```

We need to select a host port that the admission webhook server will bind to. Instead of choosing a port randomly, we should select from a designated pool so that we can keep track of these ports. We can start with `9400`.

#### Steps
* Grant the `ServiceAccount` of the `DaemonSet` access to the `hostnetwork` `SCC`, apply the following `RBAC`. 
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: clusterresourceoverride-scc-hostnetwork-use
  namespace: cluster-resource-override
rules:
  - apiGroups:
      - security.openshift.io
    resources:
      - securitycontextconstraints
    verbs:
      - use
    resourceNames:
      - hostnetwork
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: clusterresourceoverride-scc-hostnetwork-use
  namespace: cluster-resource-override
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: clusterresourceoverride-scc-hostnetwork-use
subjects:
  - kind: ServiceAccount
    namespace: cluster-resource-override
    name: clusterresourceoverride
```

* Grant `create` verb on the designated API resource of the API group the admission webhook serves to `system:anonymous`.
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: clusterresourceoverride-anonymous-access
rules:
  - apiGroups:
      - "admission.autoscaling.openshift.io"
    resources:
      - "clusterresourceoverrides"
    verbs:
      - create
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: clusterresourceoverride-anonymous-access
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: clusterresourceoverride-anonymous-access
subjects:
  - apiGroup: rbac.authorization.k8s.io
    kind: User
    name: system:anonymous
```

* Deploy the `DaemonSet`
```yaml
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: clusterresourceoverride
  labels:
    clusterresourceoverride: "true"
spec:
  selector:
    matchLabels:
      clusterresourceoverride: "true"
  template:
    metadata:
      name: clusterresourceoverride
      labels:
        clusterresourceoverride: "true"
    spec:
      nodeSelector:
        # we want the pods to be running on every master node.
        node-role.kubernetes.io/master: ''

      # enable hostNetwork to do localhost serving
      hostNetwork: true

      serviceAccountName: clusterresourceoverride
      containers:
        - name: clusterresourceoverride
          image: docker.io/autoscaling/clusterresourceoverride:dev
          imagePullPolicy: Always
          args:
            # the server binds to 127.0.0.1 to disable external connection.
            # pod readiness and liveness check does not work.
            - "--bind-address=127.0.0.1"
            - "--secure-port=9400"      
            - "--audit-log-path=-"
            - "--tls-cert-file=/var/serving-cert/tls.crt"
            - "--tls-private-key-file=/var/serving-cert/tls.key"
            - "--v=8"
          env:
            - name: CONFIGURATION_PATH
              value: /etc/clusterresourceoverride/config/override.yaml
          ports:
            - containerPort: 9400
              hostPort: 9400
              protocol: TCP
          volumeMounts:
            - mountPath: /var/serving-cert
              name: serving-cert
          readinessProbe:
            httpGet:
              path: /healthz
              port: 9400
              scheme: HTTPS
      volumes:
        - name: serving-cert
          secret:
            defaultMode: 420
            secretName: server-serving-cert
      tolerations:
        - key: node-role.kubernetes.io/master
          operator: Exists
          effect: NoSchedule
        - key: node.kubernetes.io/unreachable
          operator: Exists
          effect: NoExecute
          tolerationSeconds: 120
        - key: node.kubernetes.io/not-ready
          operator: Exists
          effect: NoExecute
          tolerationSeconds: 120
```


* Create a `MutatingWebhokConfiguration` object.
```yaml
apiVersion: admissionregistration.k8s.io/v1beta1
kind: MutatingWebhookConfiguration
metadata:
  name: clusterresourceoverrides.admission.autoscaling.openshift.io
  labels:
    clusterresourceoverride: "true"
webhooks:
  - name: clusterresourceoverrides.admission.autoscaling.openshift.io
    clientConfig:
      # serving on localhost.
      url: https://localhost:9400/apis/admission.autoscaling.openshift.io/v1/clusterresourceoverrides
      caBundle: SERVICE_SERVING_CERT_CA
    rules:
      - operations:
          - CREATE
          - UPDATE
        apiGroups:
          - ""
        apiVersions:
          - "v1"
        resources:
          - "pods"
    failurePolicy: Fail
```


#### SubjectAccessReview
To authorize an incoming request, the Admission Webhook server posts a `SubjectAccessReview` request to the `kube-apiserver`.
```json
{
  "kind":"SubjectAccessReview",
  "apiVersion":"authorization.k8s.io/v1beta1",
  "metadata":{
    "creationTimestamp":null
  },
  "spec":{
    "resourceAttributes":{
      "verb":"create",
      "group":"admission.autoscaling.openshift.io",
      "version":"v1",
      "resource":"clusterresourceoverrides"
    },
    "user":"system:anonymous",
    "group":[
      "system:unauthenticated"
    ]
  }
}
```
If `create` verb is not granted to `system:anonymous` on the above resource then the `kube-apiserver` will respond with the following status 
```
status":{"allowed":false}

Forbidden: "/apis/admission.autoscaling.openshift.io/v1/clusterresourceoverrides?timeout=30s", Reason: ""
```

### Risks and Mitigations

External teams may be surprised that their resource requirements are not respect, but ultimately the cluster-admin is in
control of his cluster.  This is what running self-hosted means.

## Design Details

### Test Plan

**Note:** *Section not required until targeted at a release.*

TBD, see open questions.

### Graduation Criteria

**Note:** *Section not required until targeted at a release.*

Define graduation milestones.

These may be defined in terms of API maturity, or as something else. Initial proposal
should keep this high-level with a focus on what signals will be looked at to
determine graduation.

Consider the following in developing the graduation criteria for this
enhancement:
- Maturity levels - `Dev Preview`, `Tech Preview`, `GA`
- Deprecation

Clearly define what graduation means.

#### Examples

These are generalized examples to consider, in addition to the aforementioned
[maturity levels][maturity-levels].

##### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers

##### Tech Preview -> GA 

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

##### Removing a deprecated feature

- Announce deprecation and support policy of the existing feature
- Deprecate the feature

### Upgrade / Downgrade Strategy

See open questions.  

### Version Skew Strategy

How will the component handle version skew with other components?
What are the guarantees? Make sure this is in the test plan.

Consider the following in developing a version skew strategy for this
enhancement:
- During an upgrade, we will always have skew among components, how will this impact your work?
- Does this enhancement involve coordinating behavior in the control plane and
  in the kubelet? How does an n-2 kubelet without this feature available behave
  when this feature is used?
- Will any other components on the node change? For example, changes to CSI, CRI
  or CNI may require updating that component before the kubelet.

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Drawbacks

The idea is to find the best form of an argument why this enhancement should _not_ be implemented.

## Alternatives

Similar to the `Drawbacks` section the `Alternatives` section is used to
highlight and record other possible approaches to delivering the value proposed
by an enhancement.

## Infrastructure Needed [optional]

Use this section if you need things from the project. Examples include a new
subproject, repos requested, github details, and/or testing infrastructure.

Listing these here allows the community to get the process for these resources
started right away.
