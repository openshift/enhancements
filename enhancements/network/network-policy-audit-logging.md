---
title: audit-logging-of-network-policy-events
authors:
  - "@astoycos"
reviewers:
  - "@abhat" 
  - "@vpickard"
  - "@trozet" 
  - "@Billy99"
approvers:
  - "@knobunc"
creation-date: 2020-12-11
last-updated: 2020-12-11
status: implementable

---

# Audit Loggging of Network Policy Events

## Release Signoff Checklist

- [X] Enhancement is `provisional`
- [X] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

The OVN-Kubernetes network type uses [OVN](https://www.ovn.org) to implement node overlay networks for Kubernetes. When OVN-Kubernetes is used as the network type for an Openshift cluster, OVN ACLs are used to implement Kubernetes' network policies (`NetworkPolicy` resources).  ACL's can either allow or deny traffic by matching on packets with specific rules. Built into the OVN ACL feature is
the ability to specify logging for each "allow" or "deny" rule.  This enhancement will activate the OVN ACL feature logging and allow the customer to manipulate the logging level, rate, and namespaces in which it is used, thereby showing valuable realtime information involving network policies.

## Motivation

Many customers require the ability to audit network policy related traffic events for regulatory and security policy compliance. Openshift currently does not have any features that satisfy this requirement. The ACL audit logging will allow customers to monitor NetworkPolicy events and identify patterns involving malicious activity with both allow and deny events. This is necessary in scenarios where
customers require a certain level of compliance, such as monitoring firewall activity, intrusion detection support, or to perform post-mortem analysis.

### Goals

- Activate configurable ACL allow/deny logging in OVN-Kubernetes on a per namespace basis.
- Allow the cluster administrator configure the auditing via the Cluster Network Operator's configuration.
- Allow the cluster administrator set the logging level via a namespace's `.metadata.annotations` field.
- Collect the relevant data from the ovn-controller logs and present it to the cluster administrator.  
- Alow the cluster administrator to extract the logs with an `oc adm node-logs --path=ovn/` command

### Non-Goals

- Network Policy Object Logging, `oc describe <entity>` takes care of that.

## Proposal

To begin implementing ACL audit logging, first the [network.operator.openshift.io](https://github.com/openshift/api/blob/master/operator/v1/types_network.go) API needs to be updated.  These changes involve adding the optional `PolicyAuditConfig` struct to the `OVNKubernetesConfig` struct as follows.

```go
// ovnKubernetesConfig contains the configuration parameters for networks
// using the ovn-kubernetes network project
type OVNKubernetesConfig struct {
    // mtu is the MTU to use for the tunnel interface. This must be 100
    // bytes smaller than the uplink mtu.
    // Default is 1400
    // +kubebuilder:validation:Minimum=0
    // +optional
    MTU *uint32 `json:"mtu,omitempty"`
    // geneve port is the UDP port to be used by geneve encapulation.
    // Default is 6081
    // +kubebuilder:validation:Minimum=1
    // +optional
    GenevePort *uint32 `json:"genevePort,omitempty"`
    // HybridOverlayConfig configures an additional overlay network for peers that are
    // not using OVN.
    // +optional
    HybridOverlayConfig *HybridOverlayConfig `json:"hybridOverlayConfig,omitempty"`
    // ipsecConfig enables and configures IPsec for pods on the pod network within the
    // cluster.
    // +optional
    IPsecConfig *IPsecConfig `json:"ipsecConfig,omitempty"`

    <--- BEGIN NEW CODE --->
    // PolicyAuditConfig is the configuration for network policy audit events. If unset,
    // reported defaults are used.
    // +optional
    PolicyAuditConfig *PolicyAuditConfig `json:"policyAuditConfig,omitempty"`
    <--- END NEW CODE --->
}
```

while the `PolicyAuditConfig` struct contains the following fields:

```go
type PolicyAuditConfig struct { 
    // RateLimit is the approximate maximum number of messages to generate per-second per-node. If
    // unset the default of 20 msg/sec is used
    // +optional
    RateLimit *uint32 `json:"rateLimit,omitempty"`

    // MaxFilesSize is the max size an ACL_audit log file is allowed to reach before rotation occurs 
    // Default is 50MB
    // +optional 
    MaxFileSize *uint32 `json:"maxFileSize,omitempty"`

    // Messages are output in syslog format. Destination is the destination for policy log messages. 
    // Regardless of this config logs will always be dumped to ovn at /var/log/ovn/ however 
    // you may also configure additional output as follows. 
    // Messages are output in syslog format.
    // Valid values are:
    // - "libc" -> to use the libc syslog() function of the host node's journdald process 
    // - "udp:host:port" -> for sending syslog over UDP
    // - "unix:file" -> for using the UNIX domain socket directly 
    // - "null" -> to discard all messages logged to syslog
    // The default is "null"
    // +optional
    Destination string `json:"destination,omitempty"`

    // SyslogFacility the RFC5424 facility for generated messages, e.g. "kern". Default is "local0"
    // +optional
    SyslogFacility string `json:"syslogFacility,omitempty`
}

```
The `aclLoggingRateLimit` field defaults to `20` messages per second.

The `MaxFileSize` field defaults to `50000000` bytes and controls how large the `/var/log/ovn/acl-audit-log.log` file is allowed to get before file rotation occurs

The `Destination` field defaults to `null` which means no acl audit logs are pushed to the host's `journald` process, if set to `libc` the host node's
`journald` process will also receive any acl-audit logs. This flag can also be configured to send the audit logs to a udp port or unix file.

The `syslogFacility` field defaults to `local0` but can configured to any RFC5424 facility

This will result in an updated OVNKubernetes Operator configuration object: `(.spec.defaultNetwork.ovnKubernetesConfig)` which will enable cluster-wide configuration of ACL logging at cluster installation time:

```yaml
spec:
  defaultNetwork:
    type: OVNKubernetes
    ovnKubernetesConfig:
      mtu: 1400
      genevePort: 6081
      ipsecConfig: {}
      PolicyAuditConfig:
        RateLimit: 20 
        MaxFileSize: 50000000 ## 50MB
        Destination: null 
        SyslogFacility: local0
```

To enable the ACL logging on a per namespace basis using metadata annotations in the namespace's definition like the following

```yaml
kind: Namespace
apiVersion: v1
metadata:
  name: Namespace-Enable-Logging
  annotations:
    k8s.ovn.org/acl-logging: '{ "deny": "alert", "allow": "notice" }'
```

or

```yaml
kind: Namespace
apiVersion: v1
metadata:
  name: tenantB
  annotations:
    k8s.ovn.org/acl-logging: '{ "deny": "notice" }'
```

The logging can be activated for either allow, drop, or allow and drop actions. The severity must be one of `alert`, `warning`,
`notice`, `info`, or `debug` as described in [OVN documentation](http://www.openvswitch.org/support/dist-docs/ovn-nbctl.8.html).

### User Stories

#### Story 1

As an Openshift user I want to see what Network Policies and accompanied ACLs are dropping the most traffic in a cluster. For example, when a packet runs into a `drop` ACL it will log a message similar to the following:

 ```sh
  2021-01-05T17:34:02.675Z|00004|acl_log(ovn_pinctrl0)|INFO|name="<Network Policy Name>", verdict=drop, severity=info: icmp,vlan_tci=0x0000,dl_src=50:54:00:00:00:02,dl_dst=50:54:00:00:00:01,nw_src=192.168.0.3,nw_dst=192.168.0.2,nw_tos=0,nw_ecn=0,nw_ttl=64,icmp_type=8,icmp_code=0
 ```

#### Story 2

As an Openshift user I want to control the rate of ACL logging to ensure I can extract the useful information involving accept/reject actions in high hit rate scenarios.

#### Story 3

As an Openshift user I want to monitor in realtime traffic that is hitting any network policies I have implemented.

#### Story 4

As an Openshift user I want to be able to extract the ACL audit logs using a command similar to viewing the API audit logs

`oc adm node-logs --role=master --path=openshift-apiserver/`

#### Story 5

As an Openshift user I want fine grain control over the ACL audit logging configuration

## Design Details

### Implementation Details/Notes/Constraints

The following codebase changes will need to done before this feature becomes usable:

- Openshift API
  - Add the necessary updates to the `OVNKubernetesConfig struct`

  Tracking PR:
  - https://github.com/openshift/api/pull/853

- OVN Kubernetes

  - Move from a cluster wide namespace default deny port group to namespace specific ones.
  - Add logging rate-limit flag for the ovnkube master process and custom OVN meter to all created ACLs:

   ```sh
    fmt.Sprintf("log=%v", aclLogging != ""),
    fmt.Sprintf("severity=%s", getACLLoggingSeverity(aclLogging)), "meter=acl-logging",
   ```
  Tracking PR's:
  - https://github.com/ovn-org/ovn-kubernetes/pull/1654
    - Main changes (Already Merged)
  - https://github.com/ovn-org/ovn-kubernetes/pull/2064
    - Minor naming changes for correlation between ACL and network Policy

- Cluster Network Operator
  
  - Digest the the `PolicyAuditConfig` struct and associated fields
  - Start OVN-K with the configured values

  - Tracking PR's:
    - https://github.com/openshift/cluster-network-operator/pull/993

### Risks and Mitigations

- How do we control who is able to see the audit logs?
  - Only Admins should be able to see the audit logs since it could help expose holes in a cluster's network policy structure

- Adding meters, which can be used to limit the logging rate, to each ACL does add a slightly larger overhead in OVN, but based on early upstream testing with logging both enabled and disabled, there were negligible performance impacts detected.

### Open Questions

1. How should the ACL logs be digested from the ovn-controller logs and presented to the user?

- Currently in OCP API audit logging is dumped directly onto the Nodes. should ACL logging do the same?
  - OCP's logging stack could be used to pull the logs and present them to the user with a Kiabana Dashboard, However the logging
    stack is an optional feature in an OCP cluster.
  - CURRENT ANSWER: Yes we also will persist the audit logs on the node at `/var/log/ovn`.

2. Based on the solution to question 1.:

- Should a must-gather collect these logs, or should they be accessible only at runtime?
- CURRENT ANSWER: Yes must gather should collect all audit logs located at `/var/log/ovn`.

3. Can the `PolicyAuditConfig` values be configured at runtime or only at cluster install?

- I think it will be possible to make them configurable at runtime however I need to complete some testing and get feedback on if
  this will be a valid use case or not.

### Future work

- To help the customer better understand what network policies/ pods are seeing the most traffic, the source/destination podname and namespace should also be included in the audit log, otherwise they'll need to manually resolve the IP which is potentially valid for a short period of time.

- A connection to the Openshift console should eventually be built, i.e a visual correlation of ACLs to network policies, pods, etc

- A custom Kibana dashboard within Openshift Logging should be constructed to query all ACL audit logs across the cluster
