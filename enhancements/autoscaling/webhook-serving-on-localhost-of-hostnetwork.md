---
title: webhook-serving-on-localhost-of-hostnetwork
authors:
  - "@tkashem"
reviewers:
  - "@deads2k"
  - "@derekwaynecarr"
approvers:
  - "@derekwaynecarr"
  - "@deads2k"
creation-date: 2019-12-04
last-updated: 2019-12-04
status: implementable
see-also:
replaces:
superseded-by:
---

# Admission Webhook Serving on localhost of hostnetwork  
We are converting api plugins into `Admission Webhook` server(s). This proposal is about how we can enable `kube-apiserver` to communicate with admission webhook server(s) over `localhost` of `hostnetwork`. 

## Release Signoff Checklist
- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary
Enable `localhost` communication over `hostnetwork` between `kube-apiserver` and admission webhook server(s).

## Motivation
* The set of admission webhook server(s) that fall into this category should follow the same pattern to be consistent across the cluster.

### Goals
* The `Admission Webhook` server(s) should be reachable to the `kube-apiserver` over `localhost` of `hostnetwork`.
* Communication between `kube-apiserver` and the webhook server must be secure using `x509`. 
* The `Admission Webhook` server(s) should only allow connection over `localhost`, thus it would disallow external connection(s).

### Non-Goals
> 1. API aggregation, make the admission webhook server reachable via the `kubernetes.default.svc` service.
> 2. It works on upstream.

## Open Questions
This is where to call out areas of the design that require closure before deciding
to implement the design.  For instance, 
 > 1. Is it possible to register the `localhost` serving admission webhook server as a `Service`? Imagine the ImagePolicy admission plugin which we want for `kube-apiserver` and `openshift-apiserver`. It doesn't have to be now and we don't want it for `ClusterResourceOverride` admission webhook.
 > 2. We are not getting the advantages to registering the webhook server as an aggregated API. See https://kubernetes.io/blog/2018/01/extensible-admission-is-beta for more information.

## Proposal
* Use `DaemonSet` to deploy the pods of the `Admission Webhook` server.
* Use `hostnetwork` for `PodSpec`, set `hostnetwork: true`.
* Select a host port that is available.
* The pod(s) must be scheduled on to each master node so that `kube-apiserver` can access the webhook server.
* The `Admission Webhook` generates `localhost` serving certs. 
* The Admission Webhook binds to `127.0.0.1` to disable external connection.
* `kube-apiserver` uses `https://localhost` to access the admission webhook server. `clientConfig.URL` of the `MutatingWebHookConfiguration` or `ValidatingWebhookConfiguration` should be set to `https://localhost:{host port}/apis/{api path}`.

### Implementation Details/Notes/Constraints
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

