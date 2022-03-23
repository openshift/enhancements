---
title: ingress-host-network-configurable-ports
authors:
  - "@elbehery"
  - "@sherine-k"
reviewers:
  - "@alebedev87"
  - "@frobware"
  - "@Miciah"
approvers:
  - "@knobunc"
  - "@tjungblu"
creation-date: 2022-01-10
last-updated: 2022-02-02
status: implementable
---

# Ingress Host Network Configurable Ports

## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement aims to enable reuse of the same nodes by 2 or more Ingress Controllers with `HostNetwork` endpoint strategy without running into port reuse conflicts. 
This is done by allowing configuration of the ports used by the router pods within the Ingress Controller API specification.

## Motivation

In the current implementation, Ingress Controllers in `HostNetwork` strategy use standard ports for HTTP (80), HTTPS (443), and stats (1936) on the underlying nodes. 

As a result, each Ingress Controller needs to be deployed on its own set of cluster nodes. It is impossible to reuse the same nodes for two Ingress Controllers.

For use cases such as multi-tenancy, multi-stage clusters, this results in increasing the number of cluster nodes, not to meet performance requirements, but for the sole purpose of having nodes with available http, https and stats ports for a new Ingress Controller.

Some clients also need to use the same nodes as infra and master combined. In such cases, it won't be possible to use port 443 for master api service as it is used by the router pod(s). In such a case, it could be helpful to give the cluster admin the ability to specify the port used by the ingress controller. 

### Goals

The goal of this enhancement is to allow cluster nodes to host router pods for more than one Ingress Controller with `HostNetwork` endpoint strategy. 

This is achieved by allowing each Ingress Controller to run on a different set of ports, thus solving the issue of concurrency on the same ports of a cluster node.

The Ingress Controller's port number become configurable in the API specification for protocols:
- HTTP
- HTTPS
- Stats

### Non-Goals

- Allowing configuration of custom ports on the ingress controllers that utilise any endpoint publishing strategy other than `HostNetwork`.

## Proposal

Configuring http, https and stats port for the ingress controllers that use the `HostNetwork` endpoint publishing strategy.


Update the IngressController API by adding port configuration fields in `HostNetwork` field:

```go
    // HostNetworkStrategy holds parameters for the HostNetwork endpoint publishing
    // strategy.
    type HostNetworkStrategy struct {
        // ...
        // HTTPPort is the port on the host which should be used to listen for
        // HTTP requests.
        // Define this field when default port 80 is known to be used by another process.
        // Please avoid specifying a port within the nodeport range, which defaults to 
        // 30000-32767, but could be configured differently.
        // +kubebuilder:validation:Optional
        // +kubebuilder:validation:Maximum=65535
        // +kubebuilder:validation:Minimum=1
        // +kubebuilder:default=80
        HTTPPort int32 `json:"httpPort"`

        // HTTPSPort is the port on the host which should be used to listen for
        // HTTPS requests.
        // Define this field when default port 443 is known to be used by another process. 
        // Please avoid specifying a port within the nodeport range, which defaults to 
        // 30000-32767, but could be configured differently.
        // +kubebuilder:validation:Optional
        // +kubebuilder:validation:Maximum=65535
        // +kubebuilder:validation:Minimum=1
        // +kubebuilder:default=443
        HTTPSPort int32 `json:"httpsPort"`

        // StatsPort is the port on the host where the stats from the router are
        // published.
        // Please avoid specifying a port within the nodeport range, which defaults to 
        // 30000-32767, but could be configured differently
        // If you configure an external load-balancer to forward 
        // connections this ingress controller, the load balancer should 
        // use this port for health checks. 
        // A load balancer can send HTTP probes to this port on a given 
        // node with the path /healthz/ready to determine whether the 
        // ingress controller is ready to receive traffic on the node.
        // For proper operation, the load balancer must not forward traffic 
        // to a node until /healthz/ready reports ready, and the load 
        // balancer must stop forwarding within a maximum of 45 seconds 
        // after /healthz/ready starts reporting not-ready. 
        // Probing every 5 or 10 seconds, with a 5-second timeout and with
        // a threshold of two successful or failed requests to become 
        // healthy or unhealthy, respectively, are well-tested values.
        // +kubebuilder:validation:Optional
        // +kubebuilder:validation:Maximum=65535
        // +kubebuilder:validation:Minimum=1
        // +kubebuilder:default=1936
        StatsPort int32 `json:"statsPort"`
      }
```

Here's an example of a public ingress controller and an internal ingress controller with the HostNetwork strategy that run on the same nodes without port conflicts:

```yaml
    ---
    apiVersion: operator.openshift.io/v1
    kind: IngressController
    metadata:
      name: public
      namespace: openshift-ingress-operator
    spec:
      domain: foo.com
      endpointPublishingStrategy:
        type: HostNetwork
    # Without any HostNetwork field under endpointPublishingStrategy, the public IngressController will be using the default ports: 
    # 80, 443, 1936
    # More spec fields here, configuring selectors and so forth...
    ---
    apiVersion: operator.openshift.io/v1
    kind: IngressController
    metadata:
      name: internal
      namespace: openshift-ingress-operator
    spec:
      domain: private.foo.com
      endpointPublishingStrategy:
        type: HostNetwork
        hostNetwork:
          httpPort: 8080
          httpsPort: 8443
          statsPort: 8936
      # More spec fields here, configuring selectors and so forth...

    ---

```
### Validation

The port used for `spec.endpointPublishingStrategy.hostNetwork.*Port` should be greater than `1`, and lower than `30000` (to avoid conflicts with 30000-32767 nodePort services range).

The `spec.endpointPublishingStrategy.hostNetwork` should not be set if the `endpointPublishingStrategy` is not of type `HostNetwork`.

The status of the Ingress Controller should report if it wasn't able to run the requested number of replicas because of the unavailability of the requested ports on the available nodes.

### User Stories

#### As a cluster administrator, I need to set binding ports for the IngressController so that I can deploy multiple ingress controllers on the same node for the HostNetwork strategy

The cluster administrator can set the ingress controller's `spec.endpointPublishingStrategy.hostNetwork`.

#### As an OpenShift developer, I want to make sure Ingress Controller documentation is updated with this new feature concerning the HostNetwork strategy

#### As an OpenShift developer, automated e2e tests cover the case of deploying 2 ingress controllers in HostNetwork strategy to the same nodes with different sets of ports, so that I can validate the feature

#### As a Cluster Administrator, I want the IngressController status to report problems provisioning the Ingress Controller pods in HostNetwork strategy when there is a port conflict

### API Extensions

This  proposal edits `IngressController.operator.openshift.io/v1`.

### Operational Aspects of API Extensions

#### External load-balancer health checks setup

An external load balancer can send HTTP probes to the ingress controller's StatsPort on a given node with the path /healthz/ready to determine whether the ingress controller is ready to receive traffic on the node. 
Sending health check requests every 5 or 10 seconds, with a 5-second timeout and with a threshold of two successful or failed requests to become healthy or unhealthy, respectively, are well-tested values.
For proper operation, the load balancer must not forward traffic to a node until /healthz/ready reports ready, and the load balancer must stop forwarding within a maximum of 45 seconds after /healthz/ready starts reporting not-ready. 

#### Investigating issues

Cluster administrators should be aware of the host ports that are used by the OpenShift cluster. These ports are referenced in the [host-port registry](https://github.com/openshift/openshift-docs/blob/main/modules/installation-network-user-infra.adoc). 

More generally, they should watch out for the ingress controller status after updates:

In case the router pod cannot be scheduled due to port unavailability (or any other scheduling concerns), the ingress controller's conditions will report the scheduling failure.

In such a case, the cluster administrator might need to inspect other pods and system daemonsets that use the same port, using `ss` or `netstat`. Router pod logs, OVN/SDN logs might also be helpful to identify the root cause for scheduling failure.

Cluster administrators might also encounter issues if an ingresscontroller with HostNetwork strategy is using a port from the nodePort range: if a nodeport service attempts to use the same port, the router pod will not be able to get scheduled correctly. The nodePort ingress controller's conditions should report the scheduling failure.


### Implementation Details

Implementing this needs changes in the following repositories:

* openshift/api
* openshift/cluster-ingress-operator

OpenShift's ingress operator creates a deployment for router pods with environment variables for port bindings, which OpenShift router already respects:
`ROUTER_SERVICE_HTTP_PORT`, `ROUTER_SERVICE_HTTPS_PORT`, `STATS_PORT`.

Also the deployments should not have ip:port conflicts for `SNI_PORT` and `NO_SNI_PORT` since [4.9](https://github.com/openshift/router/pull/326):

The router gets rid of the loopback hop  by using a Unix domain socket instead of TCP sockets to connect the be_sni backend to the fe_sni frontend and the be_no_sni backend to the fe_no_sni frontend.

### Risks and Mitigations

#### Failure Modes

* A router pod could fail to be scheduled if no node had compatible labels and taints and available ports; in this case, the ingresscontroller's status conditions should report the scheduling failure.
* A router pod could be scheduled to a node where the configured port were in use by something other than a pod, such as a system daemon (the Kubernetes schedule only checks for conflicts with ports allocated by Kubernetes); in this case, the ingresscontroller's status conditions should reflect the pod's failure to start.
* A router pod could be scheduled and start listening on a port inside the nodeport range; in this case, a subsequently created nodeport service could by chance be assigned the in-use port and fail to be configured on the host with the router pod.

#### Support Procedures

a. Checking ingresscontroller's status conditions

```sh
oc get IngressController -n openshift-ingress-operator
```
In case of a failure to schedule the router pods, the result should at least show, in the status conditions:
```yaml
  - lastTransitionTime: "2022-01-05T12:01:32Z"
    status: "False"
    type: PodsScheduled
```

b. Checking router pod logs
```sh
oc get pods -n openshift-ingress
# get the pod name
oc logs pods/pod_name -n openshift-ingress
```
Logs reporting the failure to bind to the requested port on the underlying host should be found.

c. Checking SDN/OVN logs
Check the logs from the ovn/sdn pods on the nodes where the router pods are running for errors related to port conflicts.

SDN:
```sh
oc logs -f ds/sdn -c sdn -n openshift-sd
E0228 16:22:00.629556    5050 proxier.go:1303] "can't open port, skipping it" err="listen tcp4 :32634: bind: address already in use" port="\"nodePort for ***\" (:32634/tcp4)"
```
OVN:
```sh
oc logs -f ds/ovnkube-node -n openshift-ovn-kubernetes -c ovnkube-node
W0301 17:17:48.060076    5734 port_claim.go:119] PortClaim for xxxx: xxxx/xxxxxxxx on port: 32634, err: listen tcp :32634: bind: address already in use
E0301 17:17:48.060097    5734 port_claim.go:145] Error claiming port for service: xxxx/xxxxxxxx: listen tcp :32634: bind: address already in use
```

When the conditions or logs show a conflict in port usage, the solution is to use a different, free port. 

#### Port Conflict

When the ports configured for the IngressController are already in use on the underlying host, the IngressController will not be able to reach the requested number of replicas. 

The reconciliation loop of the Cluster Ingress Operator is responsible for returning a comprehensive error message in the ingressController's status about the conflicting port(s).

#### Usage of X-Forwarded headers

By setting the `httpHeaders` in the IngressController specification, you specify when and how the Ingress controller sets the `Forwarded`, `X-Forwarded-For`, `X-Forwarded-Host`, `X-Forwarded-Port`, `X-Forwarded-Proto`, and `X-Forwarded-Proto-Version` HTTP headers.

By default, the policy is set to `Append`, which means the Ingress Controller preserves any existing headers, and appends information in the headers as needed.

In case this policy has any side effects on the customer's application behavior, the policy of the ingress controller can be modified as per [documentation](https://docs.openshift.com/container-platform/4.9/networking/ingress-operator.html#nw-using-ingress-forwarded_configuring-ingress).

## Design Details


### Test Plan


### Graduation Criteria

N/A.

#### Dev Preview -> Tech Preview

N/A.  

#### Tech Preview -> GA

N/A.  

#### Removing a deprecated feature

N/A. 

### Upgrade / Downgrade Strategy

Although there shouldn't be any issues when upgrading to a version of OpenShift with this enhancement, users must first manually remove any ingresscontrollers using this enhancement before downgrading, redirecting traffic to other ingresses for example.

### Version Skew Strategy

N/A.

## Implementation History

N/A.

## Drawbacks

Additional operational complexity, which will increase the needed test matrix, frustration for users, and volume of customer cases.

## Alternatives

* Advise users to use the NodePortService endpoint publishing strategy type. 
* Augment the NodePortService strategy to allow the user to specify requested ports within the nodeport range. 

In OpenShift 3, specifying the ports used in HostNetwork strategy  was easy to achieve, and many customers have used this feature.

The above alternatives have been rejected by a subset of users who require HostNetwork for unspecified reasons. These customers still consider the absence of this feature as a blocker/version gap. In case of migrations, customers expect making the least changes possible. They would for example prefer not to make changes around firewall setups.

Also, NodePortService with `externalTrafficPolicy` set to `cluster` comes with some overhead resulting from source address translation, which is not necessary in the case of HostNetwork strategy.