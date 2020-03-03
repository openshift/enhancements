---
title: Enabling ProjectRequestLimit on 4.x
 
authors:
  - "@akashem"

reviewers:
  - "@sttts"
  - "@deads"
  
approvers:
  - "@sttts"
  - "@deads"    

creation-date: 2020-03-03

last-updated: 2020-03-03

status: provisional

see-also:

replaces:

superseded-by:

---

# Enabling ProjectRequestLimit on 4.x
The `requestlimit.project.openshift.io/ProjectRequestLimit` admission plugin is not enabled in 4.x. The plugin already exists, this design 
is about how we make it possible for a customer to enable it on a 4.x cluster.

## Release Signoff Checklist
- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift/docs]

## Summary
The `requestlimit.project.openshift.io/ProjectRequestLimit` admission plugin is used to impose a limit on the number of
self-provisioned project(s) requested by a given user. In 3.x, the limit can be specified via the master configuration file.
It doesn't fit well with our targeted configuration which aims to avoid adding lots of intricately documented knobs. 
This is why this admission plug-in is disabled in OpenShift 4.x. 

Instead of wiring the admission plugin via the `openshift-apiserver` operator, we can create a validating admission webhook 
based on the [generic-admission-server](https://github.com/openshift/generic-admission-server) and distribute it via
OperatorHub/OLM.

## Motivation
* Some customers use the `ProjectRequestLimit` plugin in 3.x and thus are blocked on their path to OpenShift 4.x. Enabling
 it on 4.x will unblock these customers.
* The `openshift-apiserver` (like any other apiserver in the system) is designed to be extended using validating/mutating 
  admission webhook(s), we have the technology to easily build one, and we have the ability to create a simple operator to manage it.
* The cluster admin can optionally enable the validating webhook on a cluster and configure limits via a custom
resource. This will sever the "legacy" link between the plugin and the `openshift-apiserver`.
* We can provide the customer with seamless install and automatic upgrades by shipping the operator via OperatorHub 
  independent of OpenShift release cycle.   


### Goals
1. Enable the `requestlimit.project.openshift.io/ProjectRequestLimit` admission plugin.
2. Use existing extension points, libraries, and installation mechanisms in the manner we would recommend to external teams.
3. Have a fairly straightforward way to install and enable this admission plugin.

### Non-Goals
1. Revisit how `requestlimit.project.openshift.io/ProjectRequestLimit` works. We want to lift it as is.
2. Couple a slow moving admission plugin to a fast moving `openshift-apiserver`.
3. Since `Project` type is owned by `openshift-apiserver`, rebootstrapping is not applicable.

### Open Questions
1. How do we describe version skew limitations to OLM so our operator gets uninstalled *before* an illegal downgrade or
 upgrade? This is a concrete case of the API we want to use isn't available before 1.16 and after 1.18, the previous API could be gone.

## Proposal
1. Create a `Validating` admission webhook server that provides `requestlimit.project.openshift.io/ProjectRequestLimit`.
2. The admission webhook will be fronted by a `Service`.
3. The webhook server is reachable through the `kube-apiserver` via the `kubernetes.default.svc` service. 
2. Create an operator that manages all lifecycle aspects of the admission webhook via a custom resource.
3. Productize the operator and ship it as a `RedHat operator` via `OperatorHub`. 
4. Provide official documentation on how to interact with the `ProjectRequestLimit` operator.

### User Stories [optional]
*Story 1*: As a cluster admin I want to enable `ProjectRequestLimit` on an OpenShift 4.x cluster so that I can limit creation of `Project`.

*Story 2*: As a cluster admin I want to remove `ProjectRequestLimit` from my cluster.

*Story 3*: As a cluster admin I want to be able to install `ProjectRequestLimit` in a disconnected environment.

*Story 4*: As a cluster admin I want my cluster to automatically upgrade to the new version of `ProjectRequestLimit` when available.

*Story 5*: As a cluster admin I want to be able to specify (at any time) limits on the number of user provisioned `Project(s)`.

Notes:
* Port `ProjectRequestLimit` plugin from 3.11 into a `Validating` admission webhook.
* The source code is here: https://github.com/openshift/origin/blob/release-3.11/pkg/project/apiserver/admission/requestlimit/admission.go
* Use  [generic-admission-server](https://github.com/openshift/generic-admission-server) to wire the validating admission webhook server. 
* The operator is OLM enabled.
* Ship `ProjectRequestLimit` as a RedHat operator via OperatorHub.
* For productization, if we go through ART, integration work should target 4.5.
* The operator should wire the OLM manifests accordingly to facilitate disconnected install (`relatedImages`).


### Implementation Details/Notes/Constraints

#### The Operand
We build the webhook admission server to also be an extension API server, thus it will enable us to aggregate it as a normal
API server. An `APIService` object named `v1.projectrequestlimits.admission.project.openshift.io` makes the API group 
`v1.projectrequestlimits.admission.project.openshift.io/v1` available within and outside of the cluster via API aggregation 
of `kube-apiserver`. The group can be reached at `/apis/projectrequestlimits.admission.project.openshift.io/v1/validatingadmissionreviews` 
of the `kube-apiserver`, i.e. via the `kubernetes.default.svc` service hostname inside the cluster. Below, we show how 
we can achieve this: 

* The admission webhook will be fronted by a `Service` named `admission-server`, as shown below.
```yaml
apiVersion: v1
kind: Service
metadata:
  namespace: openshift-project-request-limit-operator
  name: admission-server
  annotations:
    service.alpha.openshift.io/serving-cert-secret-name: server-serving-cert
spec:
  selector:
    projectrequestlimits.operator.project.openshift.io/admission-server: "true"
  ports:
  - port: 443
    targetPort: 8443
```
 
* We define an `APIService` to register the aggregated API provided by the webhook:
```
apiVersion: apiregistration.k8s.io/v1
kind: APIService
metadata:
  name: v1.projectrequestlimits.admission.project.openshift.io
spec:
  group: projectrequestlimits.admission.project.openshift.io
  version: v1

  service:
    name: admission-server
    namespace: openshift-project-request-limit-operator
``` 

* We define a `ValidatingWebhookConfiguration` that will allow other components  (`openshift-apiserver` for one) 
  to reach the webhook via the registered aggregated API:
```
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingWebhookConfiguration
webhooks:
- clientConfig:
    service:
      namespace: default
      name: kubernetes
      path: /apis/projectrequestlimits.admission.project.openshift.io/v1/validatingadmissionreviews
    caBundle: KUBE_CA
```

`openshift-apiserver` reaches out to the admission webhook via `kubernetes.default.svc` as defined in the `ValidatingWebhookConfiguration`.
`kube-apiserver` reaches out to the webhook via the `admission-server` service in the `openshift-project-request-limit-operator` 
namespace. At a high level, the call chain looks as below:
```
    Project request -> openshift-apiserver -> kube-apiserver -> ProjectRequestLimit webhook.
```

If we do a deep dive, the call chain to the admission webhook looks as below:
```
Project request -> kube-apiserver -> kube aggregator layer inside the kube-apiserver ->
   openshift-apiserver -> admission layer in openshift-apiserver -> kube-apiserver -> 
     kube-aggregator layer inside the kube-apiserver -> ProjectRequestLimit webhook 
```

The webhook server will have the following topology:
* The webhook is deployed as a `DaemonSet` server.
* We restrict the webhook server to the master nodes only. Apply appropriate `nodeSelector` and `tolerations` to the 
  `PodSpec` so that the `Pods` are scheduled on to the master nodes.


#### The Operator
The operator will allow a cluster admin to:
* Enable or disable `ProjectRequestLimit` admission webhook on a cluster.
* Specify `limits` on `Project` create request(s) that the `ProjectRequestLimit` admission webhook can enforce.
* Manage other lifecycle aspects of the operand.

The operator will define a CRD to expose its API. The cluster admin will interact with the operator via a corresponding custom resource:
* We treat the `ProjectRequestLimit` admission webhook (represented by a `DaemonSet`) as a cluster singleton. That means
  the operator needs to manage a single deployment (specified via a `DaemonSet`) of the operand .
* For the above reason, the CRD will be defined as `cluster-scoped`, and
* The operator will be reconciling a CR named `cluster`, it will ignore other custom resources.

```
apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  name: projectrequestlimits.operator.project.openshift.io
spec:
  group: operator.project.openshift.io
  version: v1
  scope: Cluster
  names:
    plural: projectrequestlimits
    singular: projectrequestlimit
    kind: ProjectRequestLimit
```  

This is how a cluster-admin can enable the `ProjectRequestLimit` admission webhook on a cluster with `limits` to be enforced. 
```
apiVersion: operator.project.openshift.io/v1
kind: ProjectRequestLimit
metadata:
  name: cluster
spec:
  limits:
  // for selector level=admin, no maxProjects is specified. This means that users with this label 
  // will not have a maximum of project requests.
  - selector:
      level: admin
  
  //  for selector level=advanced, a maximum number of 10 projects will be allowed.
  - selector:
      level: advanced
    maxProjects: 10
  
  // no selector is specified. This means that it will be applied to any user that doesn’t satisfy 
  // the previous two rules. Because rules are evaluated in order, this rule should be specified last.
  - maxProjects: 2
```

##### Install:
* The operator is installed into a predefined namespace by OLM: The predefined namespace `openshift-project-request-limit-operator` 
  will be wired in to the OLM manifests.
* The operator will install the `ProjectRequestLimit` admission webhook into the same namespace as the operator. 


##### Configuration:
The operand is immutable, it loads the configuration from a file when it starts. A `ConfigMap` containing the configuration
is mounted as a volume into the Pod at a certain file path.
```
apiVersion: apps/v1
kind: DaemonSet
spec:
  template:
    metadata:
      annotations:
        projectrequestlimits.operator.project.openshift.io/configuration.hash: abcdefgh
    spec:
      containers:
        - name: project-request-limit-admission
          command:
            - /usr/bin/project-request-limit-admission
          args:
            - "--configuration=$(CONFIGURATION_PATH)"
          env:
            - name: CONFIGURATION_PATH
              value: /etc/projectrequestlimit/config/limits.yaml
          volumeMounts:
            name: configuration
            mountPath: /etc/projectrequestlimit/config/limits.yaml
            subPath: limits.yaml
      volumes:
        - name: configuration
          configMap:
            name: project-request-limit-configuration
``` 

The operator will ensure that change(s) made to the configuration in the `cluster` custom resource are propagated to the 
`ProjectRequestLimit` admission webhook immediately. So the wait interval is between the time an admin makes a change 
and pod restart throughout the `DaemonSet`.  
* The cluster admin changes the configuration inside the `cluster` custom resource.
* The operator detects the change and compares the hash of the `desired` configuration with that of the `current` configuration.
* If the configuration has changed, the operator updates the `ConfigMap` object and will set the annotation 
  `projectrequestlimits.operator.project.openshift.io/configuration.hash` of the template `PodSpec` of the operand.

Any change(s) to the secondary resources are detected and dealt with appropriately to maintain a functioning 
`ProjectRequestLimit` webhook.

##### Certs
The operator will leverage `service-ca` operator to populate the serving certs. `service-ca-operator` will rotate the 
certs before they expire. 
* The operator watches the corresponding `Secret` object that service-ca operator creates/updates.
* The operator maintains a hash of the current cert keys. 
* If the cert has changed the operator updates the annotation `projectrequestlimits.operator.project.openshift.io/cert.hash`
  of the template `PodSpec` of the operand. This causes a pod restart throughout the `DaemonSet` and the new cert is loaded.


We will leverage `service-ca` operator to annotate the `APIService` and the `ValidatingWebhookConfiguration` object. 


##### Uninstall
* When the `cluster` CR is deleted by a cluster-admin, he `ProjectRequestLimit` admission webhook and all secondary 
  resource(s) associated with it should be removed. We can hang ownership of the operand resources off of the `cluster` 
  CR which will ensure that the garbage collector can claim all resources once the CR is removed.
* Uninstalling the operator will leave the `ProjectRequestLimit` admission webhook intact. A cluster admin can uninstall 
  and reinstall the operator or an upgrade to the new version of the operator can happen. In neither case, should the 
  operand be affected. 


##### Repo
The operand and the operaotr will have their own repo in github.
* Operator: https://github.com/openshift/project-request-limit-operator
* Operand: https://github.com/openshift/project-request-limit

### Risks and Mitigations
* Part of CPaaS pipeline that we depend on is being actively worked on (it's targeted for 4.5). This poses a risk if we want
  to deliver the operator in the 4.5 timeline.

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
