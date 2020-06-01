---
title: allow-external-ip-overrides-for-services
authors:
  - "@abhat"
reviewers:
  - "@deads2k"
  - "@danwinship"
  - "@sttts"
  - "@squeed"
approvers:
  - "@deads2k"
creation-date: 2019-09-23
last-updated: 2019-09-23
status: implemented
see-also:
replaces:
superseded-by:
---

# Allow External IP Overrides for Services

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

The OpenShift API server provides a set of network admission plugins. One of these plugins is the external IP range checker.
There is an [externalIPNetworkCIDRs](https://docs.openshift.com/container-platform/3.11/install_config/master_node_configuration.html#master-node-config-network-config "externalipnetworkcidr") parameter 
that controls the allowable range of external IPs that a service can have in a cluster. 
This enhancement proposal is to modify the external IP range checker admission plugin to allow an admin user with 
sufficient privileges to be able to over-ride the range specified in the **externalIPNetworkCIDRs** parameter. 
An admin user can do so by configuring the service specifications' [externalIPs](https://kubernetes.io/docs/concepts/services-networking/service/#external-ips "externalips") field to a value that falls outside of the allowed range.

## Motivation

In 4.1, the **externalIPNetworkCIDR** config flag is missing from the network configuration. 
This combined with the fact that the external IP range check admission plugin is not registered in 4.1, creates a security 
issue that all external IPs are allowed. There is a parallel proposal to block all external IPs to plug this security hole 
in a 4.1 z-stream release. But when we block all external IPs, we also need to provide admin users the ability to specify 
the external IPs where safe.

In 4.2, the **externalIPNetworkCIDR** will be re-added as a [configuration parameter](https://github.com/openshift/api/blob/master/config/v1/types_network.go#L54) in the network-config yaml for the openshift-sdn. 
However, users who would upgrade from 4.1 z-stream where admin over-ride of external IPs is not just supported but needed 
to 4.2, would expect their service configurations to work as is. This proposal therefore is to allow the admin over-ride 
in 4.x as a whole.

### Goals

- Provide a mechanism for cluster admin user(s) to over-ride cluster configured values of ranges of external IPs for a service. 
<a name="admin-user">An admin user here is defined as someone who is given a special permission - see details below</a>.

- As a corollary, non-admin user should not be able to patch/update the service spec with external IPs that fall out of the 
range specified at the cluster level.

- Both admin and non-admin users should still be able to specify external IPs for a service that fall within the range 
specified at the cluster level.

### Non-Goals

- Escalation of privileges by causing service traffic to be routed to a malicious external IP address.

## Proposal

- As an administrator with [sufficient privilege](#admin-user) on a 4.x OpenShift cluster, I want the ability to 
specify external IPs which may fall out of the range specified by cluster administrators for services belonging to my app.

This proposal is to add an RBAC check in case the external IPs specified for the service don't fall in the range of the 
cluster specified **externalIPNetworkCIDR**. This is needed particularly in 4.1, where the external IPs will be blocked 
completely. In case, there are validation errors as a result of the specified external IP(s) not being in the valid range, 
the RBAC checker will validate that the user has permissions to create a dummy subresource for the service in question. 
If the result of that check is an Allow, the user is allowed to set the external IP(s) on the service.

Code snippet that performs the actual RBAC check:

	``` go
    func (r *externalIPRanger) checkAccess(attr admission.Attributes) (bool, error) {
            	authzAttr := authorizer.AttributesRecord {
            		User:            attr.GetUserInfo(),
            		Verb:            "create",
            		Resource:        "service",
            		Subresource:     "externalips",
            		APIGroup:        "network.openshift.io",
            		ResourceRequest: true,
            	}
            	authorized, _, err := r.authorizer.Authorize(authzAttr)
            	return authorized == authorizer.DecisionAllow, err
    }
	```

There is a precedent of doing something similar in the restricted endpoints admission controller. The endpoints admission 
controller plugin checks if the specified service endpoint is a restricted endpoint. If so, it will perform a RBAC check 
to validate that the user creating/updating the service and/or the endpoint information itself has admin privileges to 
create a subresource underneath the endpoint resource in the current namespace. 

In our case however, we will need cluster level admin privilege to override the external IPs because setting the external IP 
for a service in a given namespace attracts all traffic destined to that IP from the node as well as from other pods in other 
namespaces to that IP address. This needs to be allowed only if the user has a higher (cluster wide) admin privilege level.

### Implementation Details/Notes/Constraints

[Current PR that implements the RBAC override](https://github.com/openshift/origin/pull/23783 "PR 23783")

### Risks and Mitigations

## Design Details

### Test Plan

New e2e tests need to be added for validating the correctness of behavior 
as well as evaluating the impact or lack thereof of privilege escalation. 

Existing unit-tests for the external IP range checker need to be updated.

### Graduation Criteria

This feature doesn't add/change any API objects.

### Upgrade / Downgrade Strategy

A 4.1 service specification that has an external IP over-ridden will continue to work since we will also make this 
change in 4.2 and beyond even though 4.2+ will bring back the ExternalIPConfig into the network-config.

If a user downgrades from 4.2 or beyond to 4.1, we will likely ignore the ExternalIPConfig parameter and treat the config 
as if it's Block All External IPs.

However, if they have over-ridden external IPs in their service config, those will continue to be honored so long as the 
user has [sufficient privileges](#admin-user).

### Version Skew Strategy

## Implementation History

## Drawbacks


## Alternatives

## Infrastructure Needed [optional]

OpenShift 4.x cluster
