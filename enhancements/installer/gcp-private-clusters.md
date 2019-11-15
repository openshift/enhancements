---
title: (GCP) Internal/Private Clusters
authors:
  - "@patrickdillon"
reviewers:
  - "@abhinavdahiya"
  - "@sdodson"
approvers:
  - "@abhinavdahiya"
  - "@sdodson"
creation-date: 2019-10-12
last-updated: 2019-10-12
status: implemented
see-also:
  - "/enhancements/aws-internal-clusters.md"  
superseded-by:
  - "https://docs.google.com/document/d/1N_lbagHiuJCiOFFVpqVkenvZQESm0FWi0H3fw60Q77M"
---

# installing.internal.private.clusters.for.gcp

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [x] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift/docs]

## Summary

Many customer environments don't require external connectivity from the outside world and as such, they would prefer not to expose the cluster endpoints to public. Currently the OpenShift installer exposes endpoints of the cluster like Kubernetes API, or OpenShift Ingress to the Internet, although most of these endpoints can be made Internal after installation with varying degree of ease individually, creating OpenShift clusters which are Internal by default is highly desirable for users.

Specifically for GCP, this enhancement requires implementation of internal load balancers, which were not an initial feature on the GCP platform.

## Motivation

### Goals

Install an OpenShift cluster on GCP as internal/private so that it is only accessible from within an internal network and not visible to the public Internet.

### Non-Goals

There is no day-two operation for making an existing cluster private. No additional isolation from clusters in the network than what is provided by shared networking.

## Proposal

Installer allow users to provide a list of subnets that should be used for the cluster. Since there is an expectation that networking is being shared, the installer cannot modify the networking setup (i.e. the route tables for the subnets or the VPC options like DHCP etc.) but, changes required to the shared resources like Tags that do not affect the behavior for other tenants of the network will be made.

The installer validates the assumptions about the networking setup.

Infrastructure resources that are specific to the cluster and resources owned by the cluster will be created by the installer. So resources like load balancers, storage buckets, VM instances and firewall rules remain cluster managed.

The infrastructure resources owned by the cluster continue to be clearly identifiable and distinct from other resources.

Destroying a cluster must make sure that no resources are deleted that didn't belong to the cluster. Leaking resources is preferred over any possibility of deleting non-cluster resources.

### User Stories

#### Story 1

As an administrator, I would like to install an OpenShift cluster in my company's private network that is accessible to within the network but not on the Internet.

### Implementation Details

#### Resources Provided to the Installer

Users provide a VPC and subnets in the install config `platform` and specify an `internal` publishing strategy in the top-level `publish` attribute:

```yaml
apiVersion: v1
baseDomain: example.com
metadata:
  name: example
platform:
  gcp:
    projectID: example-project
    region: us-east4
    computeSubnet: example-worker-subnet
    controlPlaneSubnet: example-master-subnet
    network: example-network
publish: Internal
pullSecret: '{"auths": ...}'
```

##### Basedomain

`Basedomain` will continue to be used for creating a private DNS zone and the necessary records, but the requirement for a public DNS zone for the basedomain will be removed.

##### Access to Internet

The cluster still requires access to the Internet.

#### Resources created by the Installer

##### Internal Load Balancers and Instance Groups

The GCP platform was originally setup to use only network load balancers (NLBs) in order to ensure necessary health checks, but it is not possible to limit access to external load balancers based on source tags[(see GCP firewall docs)][gcp-firewall-sources]. Therefore, it is necessary to implement internal load balancers to allow access to internal instances. 

The Internal Load Balancer relies on Instance Groups rather than the Target Pools used by the NLB. The installer will create instance groups for each zone, even if there is no instance in that group. This difference will need to be taken into account by the [cluster-api-provider-gcp][gcp-provider]. 

###### Other Resources to Be Created Differently

* Cluster IP - The cluster IP address will be internal only. 
* Forwarding rule - One forwarding rule will handle both the Kubernetes API & machine config server ports.
* Backend service - Comprised of each zone’s instance group & (temporarily) bootstrap instance group
* Health check- api only (see limitations)
* DNS Record Sets - No records for the public zone need to be created. rrdatas for the remaining record sets should point to the ILB
* Firewall - combined into single rule, source ranges reduced to internal only


### Risks and Mitigations

## Design Details

### Test Plan

The containers in the CI cluster that run the installer and the e2e tests will create a VPN connection to a public VPN endpoint created in the network deployment in the CI GCP account. This will provide the client transparent access to otherwise internal endpoints of the newly created cluster.

### Graduation Criteria

This enhancement will follow standard graduation criteria.

#### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers

#### Tech Preview -> GA

- Community Testing
- Sufficient time for feedback
- Upgrade testing from 4.3 clusters utilizing this enhancement to later releases
- Downgrade and scale testing are not relevant to this enhancement

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

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

N/A

### Version Skew Strategy

N/A

## Implementation History

Implementation PR: openshift/installer#2522

## Drawbacks

A limitation of this design is that no health check for the machine config server (/healthz) will be run. The health of an instance will be determined entirely by the /readyz check on port 6443. This is mostly because two ILBs cannot share a single IP address, whereas twp NLBs can share a single external IP.

## Alternatives

Other alternatives were considered but this is the only known feasible design. As mentioned above, using external network load balancers and limiting public access with firewalls is not possible, because tagged VM instances can not be granted access through source tags. 

## Infrastructure Needed [optional]

* Network deployment in GCP CI account which includes VPC, subnets, NAT gateways, Internet gateway that provide egress to the Internet for instance in private subnet.
* VPN accessibility to the network deployment created above.

[gcp-firewall-sources]: https://cloud.google.com/vpc/docs/firewalls#sources_or_destinations_for_the_rule
[gcp-provider]: https://github.com/openshift/cluster-api-provider-gcp