---
title: kubernetes-slb-owned-api-loadbalancers
authors:
  - "@abhinavdahiya"
reviewers:
  - "@sttts"
  - "@smarterclayton"
approvers:
  - "@sttts"
  - "@smarterclayton"
creation-date: 2020-05-06
last-updated: 2020-08-26
status: implementable
see-also:
  -   
replaces:
  - 
superseded-by:
  - 
---

# Kubernetes Service Owned API Loadbalancers

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Open Questions [optional]

1. Service type Loadbalancer's ExternalTrafficPolicy should be Cluster or Local ?
2. How to reserve private IPs on AWS ?

## Summary

Currently the installer is responsible for creating the loadbalancers for API and API-INT endpoints for the cluster. These loadbalancers 
are only created at install-time and their lifecycle is not owned by the cluster. Since the Kubernetes API is hosted in pods in the cluster, 
using the Kubernetes service type `Loadbalancer` to manage the lifecycle of the loadbalancers is next-step for self-hosted Kubernetes.

## Motivation

### Goals

1. The API, API-INT loadbalancers are managed by Kubernetes service type `Loadbalancer`.
2. Recovery of the cluster from some, to *all* of the control-plane DOWN scenarios should not be substantially more difficult. We do not want 
   to trade cluster owned lifecycle for easy operations for harder recovery of control-plane.
4. Support this deployment model for all platforms that support service type `Loadbalancer`.
5. Allow customers to opt-out for user-provisioned workflows.

### Non-Goals

1. Updating existing clusters to new deployment strategy on upgrades.


## Background

Currently the bootstrapping happens in these three stages,

**Stage 1:** Bootstrapping control plane on bootstrap host

PUBLIC ZONE:api=public-lb-ip
PRIVATE ZONE:api=private-lb-ip;api-int=private-lb-ip

public-lb:bootstrap(green),master-0(red);master-1(red);master-2(red)
private-lb:bootstrap(green);master-0(red);master-1(red);master-2(red)

The `bootstrap` machine hosts the bootstrap-control-plane, and the kubelet on `master-{0,1,2}` use the private-lb to communicate with k8s API.

**Stage 2:** Bootstrapping complete

PUBLIC ZONE:api=public-lb-ip
PRIVATE ZONE:api=private-lb-ip;api-int=private-lb-ip

public-lb:bootstrap(red),master-0(green);master-1(green);master-2(green)
private-lb:bootstrap(red);master-0(green);master-1(green);master-2(green)

Bootstrapping of the k8s API is complete and therefore the bootstrap-control-plane has shut itself down on the `bootstrap` machine. Here the actual 
control-plane is serving all the request for k8s API

**Stage 3:** Bootstrap host removed

PUBLIC ZONE:api=public-lb-ip
PRIVATE ZONE:api=private-lb-ip;api-int=private-lb-ip

public-lb:master-0(green);master-1(green);master-2(green)
private-lb:master-0(green);master-1(green);master-2(green)

The installer terminates the `bootstrap` machine and also removes the `bootstrap` machine from the backend of all the loadbalancers. And only 
the `master-{0,1,2}` machines remain.

During all these stages, all the clients end up using the loadbalancers as a single abstraction and do not have to understand the bootstrapping 
stages or even notice the bootstrap-control-plane.

## Proposal

During bootstrapping the clients use the known API, and API-INT DNS names to discover the endpoints. Ensuring that the clients continue to depend on 
that abstraction is very important as it allows the clients to be simple.

Secondly, the load balancers cannot be created by Kubernetes cloud controller manager unless there is a Kubernetes API already running, but for 
successful bootstrapping we need the loadbalancers so that the kubelets can communicate with bootstrap-control-plane to run the necessary pods to create 
the k8s API.

So, the break the cyclic dependency, I suggest we modify the stages as follows,

**Stage 1:** Bootstrapping control plane on bootstrap host

PUBLIC ZONE:api=static-public-lb-ip
PRIVATE ZONE:api=bootstrap-private-ip,static-private-lb-ip;api-int=bootstrap-private-ip,static-private-lb-ip

The `bootstrap` machine hosts the bootstrap-control-plane, and the installer has pre-seeded the DNS entries with reserved ips for loadbalancers to 
be created by the cluster. The kubelet on `master-{0,1,2}` use the DNS to resolve the endpoints to the `bootstrap` machine, 
and communicate with the k8s API.

An entity generates Kubernetes service type loadbalancer objects using the static ips created by the installer and pushes them to the cluster using 
the bootstrap control plane. The bootstrap-control-plane 's kube-controller-manager creates the LBs for the services.

**Stage 2:** Bootstrapping complete

PUBLIC ZONE:api=static-public-lb-ip
PRIVATE ZONE:api=bootstrap-private-ip,static-private-lb-ip;api-int=bootstrap-private-ip,static-private-lb-ip

public-lb:master-0(green);master-1(green);master-2(green)
private-lb:master-0(green);master-1(green);master-2(green)

Bootstrapping of the k8s API is complete when the necessary pods are running, and the service type load balancers are running. At this point 
the `bootstrap-control-plane` shuts itself down. The clients can now reach the k8s API using the loadbalancer ips, the clients trying to reach API 
using `bootstrap` host will fail and round-robin to the loadbalancer ips.

**Stage 3:** Bootstrap host removed

PUBLIC ZONE:api=static-public-lb-ip
PRIVATE ZONE:api=static-private-lb-ip;api-int=static-private-lb-ip

public-lb:master-0(green);master-1(green);master-2(green)
private-lb:master-0(green);master-1(green);master-2(green)

The installer terminates the `bootstrap` machine and also removes the `bootstrap` machine's ip addresses from the DNS. And only the `master-{0,1,2}` machines
remain with only loadbalancer ips in the DNS.

### User Stories

#### Readiness logic maintained by apiserver team

Today the load balancers are created by the installer and therefore the health-checks endpoints and levels are controlled by the installer, 
but with SLB owned API loadbalancers the apiserver team can defined the readiness on the pods themselves and the k8s-networking-provider will 
bring pods in/out of backend.

#### Changing the publising strategy of API as day-2

Since the API loadbalancers are now Service objects, a user can easily remove the public facing object to remove the Internet facing access of the API,
or add Internet facing access for internal clusters.

#### API endpoints tracking

The ability of the kubelet to communicate with k8s API is very important for self hosted control-plane. Currently the kubelet uses the load balancer to 
communicate with the API, which has seen various bugs in the past causing the kubelet to loose connection to API.

To improve the resilency of the connection, we can program alternate endpoints for the API like localhost, individual pod/node ip etc, allowing the kubelets 
to use multiple paths to reach the API.

Using service type loadbalancer would allows the programming against the Endpoint objects for the Service to track the ip addresses of the pod.

### Implementation Details/Notes/Constraints

#### More DNS, Why?

One way to avoid DNS would be if the installer could continue to create the loadbalancers but then kubernetes controller manager can adopt those resources 
for the SLB object created by the installer. There are 2 major reasons why that is not possible,

- The controllers create cloud resources based on the [UID of the service object](https://github.com/kubernetes/cloud-provider/blob/master/cloud.go#L88). 
    This is deprecated method and recommends the cloud provider implementations use more useful names. There are no cloud providers that have moved to a 
    better naming scheme and there are no enhancements for such a change. The installer can only create the resources that are correctly adopted if the names 
    are predictable. And even if the installer could create these resources, there would be very high tight coupling of the implementation of controllers and 
    the installer.
- The bootstrap host needs to be part of the backend of the load balancers, but the bootstrap host is not a node in the cluster. Therefore, the controllers 
    when adopting the resources will remove the bootstrap host from the backend before the control-plane endpoints were running making the bootstrapping more
    fragile.

Using DNS allows the installer to abstract the bootstrap host and load balancer behind the same DNS and k8s controllers can own the lifecycle of SLB from 
creation to mangement. This keeps the installer's bootstrapping workflow simpler.

#### Using installer created static ips

The installer will create static ips for the loadbalancers created by the cluster for API and API-INT. This allows the installer to pre-seed the DNS records 
with ips of the `bootstrap` machine and these ips and also update the records after bootstrapping is complete to include only the loadbalancer ips.

An alternative would have been to allow the cluster to use dynamic ips and then update the records itself, but there are several caveats like,

- Accessing the cloud DNS APIs from the cluster has proven to be difficult esp. on AWS where in disconnected(air-gapped) environment the cluster 
    cannot communicate to the route53 API because of lack of VPC endpoints.
- Allowing the cluster to perform the DNS actions for such a crucial endpoint means we will be forced to support various DNS providers. Restricting 
    the DNS operations to installer allows the in-cluster manager to be simple for the time being as we understand the landscape a little bit more.

#### RRDNS and unusuable ips

During bootstrapping where the installer creates the DNS records with both the `bootstrap` host IPs and static IPs, the clients like the kubelet, 
network operator can try to connect to the static IP which is not yet routable to anything. Also, when the bootstrapping is complete and the 
bootstrap-control-plane has itself shutdown on the `bootstrap` host, again clients previously can still end up trying to connect to the bootstrap ip 
which is no longer serving traffic.

In both these situations the clients are going to see some errors for the time being, but in both these situations RRDNS should allow the clients to 
find and use the correct ip addresses for the API.

An in-cluster manager that is making sure only correct ip addresses are part of the DNS would reduce the timeframe for in-consistent behavior, 
but the [trade-off](#Using-installer-created-static-ips) of not handling the DNS in-cluster has more benefits compared to some elevated error rates 
during bootstrapping.

#### Azure reserving static IPs

Azure allows reserving static IPs for use with both Standard Loadbalancer and Standard Internal Loadbalancer. And we would need to reserve only one VIP for
the Standard Loadbalancer and one VIP from the compute subnet for the Standard Internal Loadbalancers.

#### Azure SLB objects

The external SLB would look like,

```yaml
apiVersion: v1
kind: Service
metadata:
  name: external-lb
  namespace: openshift-kube-apiserver
spec:
  loadBalancerIP: <reserved public ip>
  type: LoadBalancer
  ports:
  - port: 6443
  selector:
    app: kube-apiserver
```

The internal SLBs would look like,

```yaml
apiVersion: v1
kind: Service
metadata:
  name: internal-lb
  namespace: openshift-kube-apiserver
  annotations:
    service.beta.kubernetes.io/azure-load-balancer-internal: "true"
spec:
  loadBalancerIP: <reserved private ip>
  type: LoadBalancer
  ports:
  - port: 6443
  selector:
    app: kube-apiserver
---
apiVersion: v1
kind: Service
metadata:
  name: internal-lb-machine-config-server
  namespace: openshift-machine-config-operator
  annotations:
    service.beta.kubernetes.io/azure-load-balancer-internal: "true"
spec:
  loadBalancerIP: <reserved private ip>
  type: LoadBalancer
  ports:
  - port: 22623
  selector:
    app: machine-config-server
```

#### GCP reserving static ips

GCP allows reserving static IPs for use with both public TCP/UDP forwarding-rules and internal TCP/UDP forwarding-rules. And we would need to reserve only 
one VIP for the public TCP/UDP forwarding-rule and one VIP from the compute subnet for the internal TCP/UDP forwarding-rules. Since the 2 internal 
forwarding-rules for API-INT and IGNITION will use the same IP, this address must be created with purpose `SHARED_LOADBALANCER_VIP`.

#### GCP SLB objects


The external SLB would look like,

```yaml
apiVersion: v1
kind: Service
metadata:
  name: external-lb
  namespace: openshift-kube-apiserver
spec:
  loadBalancerIP: <reserved public ip>
  type: LoadBalancer
  ports:
  - port: 6443
  selector:
    app: kube-apiserver
```

The internal SLBs would look like,

```yaml
apiVersion: v1
kind: Service
metadata:
  name: internal-lb
  namespace: openshift-kube-apiserver
  annotations:
    cloud.google.com/load-balancer-type: "Internal"
spec:
  loadBalancerIP: <reserved private ip>
  type: LoadBalancer
  ports:
  - port: 6443
  selector:
    app: kube-apiserver
---
apiVersion: v1
kind: Service
metadata:
  name: internal-lb-machine-config-server
  namespace: openshift-machine-config-operator
  annotations:
    cloud.google.com/load-balancer-type: "Internal"
spec:
  loadBalancerIP: <reserved private ip>
  type: LoadBalancer
  ports:
  - port: 22623
  selector:
    app: machine-config-server
```

#### AWS reserving the static ips

AWS allows users to create elatic IPs which are reserved public IPs, but it does not allow reserving static private IPs from a subnet. The only way the one
can reserve an internal IP is to create a elatic network interface in the subnet. That affects their static IPs for Network Loadbalancer support, where for
pubic NLBs you need to provide elastic IP reservations for each target subnet where backends EC2 instances exists. And for internal NLBs you can provide
static IP for each target subnet but the user needs to guarantee that the specifc IP in not in use by another other resource. Since AWS only allows the users
to create network interfaces for reserve on IP on private network, it would have been better if AWS allowed to users to provide the elastic network interfaces
for the internal NLBs.

This makes reserving static IPs for cluster created loadbalancers very tricky, and highly prone to races esp in an setup where multiple clusters are being created and destroyed in the same VPC like OpenShift CI.

A tentative solution would be,
- The installer created network interfaces for the subnets to reserve a private IP
- The actor that create the Service objects lists the network interfaces, create the Service objects with the IPs and then issues delete.

This solution is very prone to race as any other network interface created between the delete and loadbalancer would possibly take that free IP.
This needs to considered when trying to use the proposed solution for AWS.

#### AWS SLB objects

AWS k8s cloud provider only support public NLBs to provide a list of elastic IPs for subnets, but does not allow providing IPs for private NLB.
There is an upstream issue tracking this support [here](https://github.com/aws/containers-roadmap/issues/894)

The external SLB would look like,

```yaml
apiVersion: v1
kind: Service
metadata:
  name: external-lb
  namespace: openshift-kube-apiserver
  annotations:
    service.beta.kubernetes.io/aws-load-balancer-type: nlb
    service.beta.kubernetes.io/aws-load-balancer-eip-allocations: "<comma separate elastic ip allocation resource IDs>"
spec:
  type: LoadBalancer
  ports:
  - port: 6443
  selector:
    app: kube-apiserver
```

#### IPv6

TODO(abhinav): Fill this up.

#### Azure internal clusters outbound

TODO(abhinav): Fill this up.

#### User provisioned workflow

The user provisioned workflow users should be allowed to use this new code-path of SLB managed load balancers, but should also be allowed to
continue to use their own loadbalancers.

The UPI workflow should document the workflow of,

- Creating the bootstrap host, and control plane hosts.
- Create IPs addresses for loadbalancers such that the bootstrap host will use those to create Service objects.
- Create DNS records with bootstrap host and IPs created above for bootstrapping and then removing the bootstrap host IP from the records.
    This will be the API-INT DNS record.

### Risks and Mitigations

TODO(abhinav): Fill this up.

## Design Details

### Azure: discover the static IPs

The actor should use the the following files on bootstrap host to decide which IPs are

- `manifests/cluster-infrastructure-02-config.yml` for config/v1 Infrastrucure object.
- `manifests/cluster-config.yaml` for InstallConfig.

These files should allow the actor to answer these questions,

- Is public IP required?
- Are IPv4 and/or IPv6 required?

TODO(abhinav): link to various enhacements that provide info on how to answer these questions.

The actor can use `openshift/99_cloud-creds-secret.yaml` file for credentials to authenticate with Azure API.

The actor on the bootstrap host should look for,

- `<infra-id>-rsvd-public-ipv4` for the public IPv4
- `<infra-id>-rsvd-private-ipv4` for the private IPv4
- `<infra-id>-rsvd-public-ipv6` for the public IPv6
- `<infra-id>-rsvd-private-ipv6` for the private IPv6

### GCP: discover the static IPs

The actor should use the the following files on bootstrap host to decide which IPs are

- `manifests/cluster-infrastructure-02-config.yml` for config/v1 Infrastrucure object.
- `manifests/cluster-config.yaml` for InstallConfig.

These files should allow the actor to answer these questions,

- Is public IP required?

TODO(abhinav): link to various enhacements that provide info on how to answer these questions.

The actor can use `openshift/99_cloud-creds-secret.yaml` file for credentials to authenticate with GCP API.

The actor on the bootstrap host should look for,

- `<infra-id>-rsvd-public-ipv4` for the public IP
- `<infra-id>-rsvd-private-ipv4` for the private IP

### Creating the loadbalancers during bootstrapping

The Service type Loadbalancer is managed by cloud provider code in kube-controller-manager. But as of today, the bootstrap-kube-controller-manager does not have
the cloud provider code-paths configured for simplicity. And since only one KCM can be active in the clsuter, the KCM running on the control-plane hosts with
necessary cloud-provider enabled is waiting for lease held by bootstrap-kcm.

For SLBs to provide loadbalancers during bootstrapping we need a kube-controller-manager with corresponding cloud-provider code-paths configured
to get the load balancers created. There are 2 ways to achive this,

1. The bootstrap-kube-controller-manager should enable the necessary cloud-provider code-paths to handle the SLBs.
    The renderer has all the information like the platform and kube-cloud-config from files on the bootstrap host to setup the cloud provider.
    The second part is the credentials to communicate with cloud APIs. Currently the KCM uses VM/Instance attached identities for credentials and we
    can attach the same identities to the bootstrap host for parity with control plane hosts.
2. The bootstrap-kube-controller-manager steps down from the lease when KCM starts running on the control-plane hosts.
    One possible way was to create priority-level for Lease and the bootstrap-kcm could be configured with lower priority allowing kcm running on the
    control plane hosts to step up as leader.
    Another possible solution would to update the cluster-bootstrap to shutdown services on the bootstrap host when their corresponding pods are in `Running`
    state. This would allow KCM on control-plane hosts to step up as leader when the bootstrap-kcm is shutdown by cluster-bootstrap
3. We enable External cloud-controller-manager that runs on the control-plane hosts to being with and can handle all SLBs.

### Updating when bootstrapping is complete

Since we want to make sure the bootstrap-control-plane API is shutdown only when SLBs have been created, we need to update the cluster-bootstrap to wait for
Services in addition to pods to decide when bootstrapping is complete.

The cluster-bootstrap would accept a list of Services using cli flag, and it would include each of the Service was `Ready` to the decision of bootstrapping complete.

Serivce `Ready` should be defined as,
- The `status.loadBalancer.ingress` has atleast one element, and
- The Endpoint for the Service has atleast one endpoint.

### Test Plan

**Note:** *Section not required until targeted at a release.*

Consider the following in developing a test plan for this enhancement:
- Will there be e2e and integration tests, in addition to unit tests?
- How will it be tested in isolation vs with other components?

No need to outline all of the test cases, just the general strategy. Anything
that would count as tricky in the implementation and anything particularly
challenging to test should be called out.

All code is expected to have adequate tests (eventually with coverage
expectations).

### Graduation Criteria

TODO(abhinav): Fill this up.

### Upgrade / Downgrade Strategy

TODO(abhinav): Fill this up.

### Version Skew Strategy

TODO(abhinav): Fill this up.

## Implementation History

None

## Drawbacks

TODO(abhinav): Fill this up.

## Alternatives

Discussed inline to the implementation.
