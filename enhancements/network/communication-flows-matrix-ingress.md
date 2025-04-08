---
title: communication-flows-matrix-ingress
authors:
  - "@sabinaaledort"
  - "@yuvalk"
  - "@liornoy"
reviewers:
  - "@trozet"
  - "@danwinship"
  - "@msherif1234"
approvers:
  - "@trozet"
  - "@danwinship"
api-approvers:
  - None
creation-date: 2023-03-07
last-updated: 2024-09-25
tracking-link:
  - https://issues.redhat.com/browse/TELCOSTRAT-77
---

# Communication ingress flows matrix of OpenShift and Operators

## Summary

This enhancement allows to automatically generate an accurate and up-to-date 
communication flows matrix that can be delivered to customers as part of 
product documentation for all ingress flows of OpenShift (multi-node and
single-node deployments) and Operators.

## Motivation

Security-conscious customers need OpenShift flows matrix for regulatory reasons 
and/or to implement firewall rules to restrict traffic to the minimum set of
required flows only, on-node firewall or external.

### User Stories

- As an OpenShift cluster administrator, I want documentation on the expected 
  flows of traffic incoming to every OpenShift installation so I can set up 
  firewall rules such as nftables, NGFW, etc. to restrict traffic to the 
  minimum required set of flows only.

- As an OpenShift cluster administrator, I want a "one-click" feature to 
  generate an accurate, relevant, matrix for the running OpenShift cluster.

- As a testing engineer, I want a machine-readable ingress traffic matrix 
  so that I can analyze and compare with tools such as nmap.

### Goals

- Provide a mechanism to automatically generate an accurate and up-to-date 
  OpenShift communication ingress flows matrix.

- Keep the communication matrix documented in OpenShift release documents
  updated and validate it does not change.

### Non-Goals
N/A

## Proposal

We introduce a new module to generate an accurate and up-to-date OpenShift 
communication ingress flows matrix.

- A communication matrix describing the expected flows of incoming traffic will 
  be included in every OpenShift release documentation.

- A new `oc` command will be added to generate a current snapshot of known 
  listening ports in a running cluster, `oc adm communication-matrix generate`.

- A new option will be added to OpenShift web console to generate an up-to-date
  communication matrix.

### Workflow Description

An OpenShift administrator would like to get an accurate and up-to-date OpenShift 
communication ingress flows matrix.

- The admin reviews OpenShift release documentation to get the included communication
  matrix describing the expected flows of incoming traffic.

- The admin uses the OpenShift command-line interface (CLI) to generate an up-to-date 
  communication matrix using the following command `oc adm communication-matrix generate`.

- Optionally, the admin uses the OpenShift web console to generate an up-to-date
  communication matrix.

### API Extensions
N/A

### Topology Considerations

#### Hypershift / Hosted Control Planes
Out of scope for this proposal.

#### Standalone Clusters

The communication matrix can be generated on standalone clusters.

#### Single-node Deployments or MicroShift

The communication matrix can be generated on single-node deployments and MicroShift.

### Implementation Details/Notes/Constraints

[commatrix-repo]: https://github.com/openshift-kni/commatrix

[commatrix-ocp-docs]: https://docs.openshift.com/container-platform/4.16/installing/install_config/configuring-firewall.html#network-flow-matrix_configuring-firewall

A new module was added to [openshift-kni/commatrix][commatrix-repo] to create a 
communication ingress flows matrix from the existing `EndpointSlices` in the 
OpenShift cluster. It supports both standard clusters and single-node deployments.

A network flow matrix, produced by the new module, has been published in the 
[official OpenShift 4.16 documentation][commatrix-ocp-docs].

In the following OpenShift releases the published matrix will be splitted by
platforms:
- Standard AWS/GCP clusters
- Standard baremetal clusters
- Single-node AWS/GCP deployments
- Single-node baremetal deployments

The module allows to generate the communication matrix in various formats:
- *YAML/JSON*: data is represented as key-value pairs
- *CSV*: a comma-separated value file
- *nftables*: rules and commands that provide firewall to control traffic and ensure security
- *AdminNetworkPolicy*: cluster-scoped resource used to secure the network by creating network policies before creating namespaces (not supported yet)

Each record describes a flow with the following information:
```
direction      Data flow direction (currently ingress only)
protocol       IP protocol (TCP/UDP/SCTP/etc)
port           Flow port number
namespace      EndpointSlice Namespace
service        EndpointSlice owner Service name
pod            EndpointSlice target Pod name
container      Port owner Container name
nodeRole       Service node host role (master/worker/other[master for SNO])
optional       Optional or mandatory flow for OpenShift
```

For example,
`ingress,TCP,6443,openshift-monitoring,prometheus-adapter,prometheus-adapter-54bb854c4f-29l6v,prometheus-adapter,worker,false`

Optionally, a user can set the `custom-entries-file` flag to describe the location
of a file with custom entires to be added to the communication matrix (can be set 
in the supported formats), for example:
```
[
  {
    "direction": "ingress",
    "protocol": "TCP",
    "port": 22,
    "service": "sshd",
    "nodeRole": "master",
    "optional": true
  },
  {
    "direction": "ingress",
    "protocol": "TCP",
    "port": 51035,
    "service": "rpc.statd",
    "nodeRole": "master",
    "optional": true
  }
]
```

The source of truth for cluster traffic used in the new module is the `EndpointSlices` 
resource. `EndpointSlices` group network endpoints together by unique combinations of 
protocol, port number, and `Service` name. For that to work, every application that 
attracts ingress traffic running in the cluster should have a `Service` object,
the control plane automatically creates `EndpointSlices` for any `Service` that 
has a selector specified. `EndpointSlices` can also be directly created when needed
or managed by other entities or controllers.

To generate the communication matrix the `EndpointSlices` in the cluster should 
be filtered by `Service` type `NodePort` and `LoadBalancer` as there are also 
`EndpointSlices` in the cluster for internal `Services`. The node hosted services 
such as sshd, rpc, etc. are currently missing `EndpointSlices` (see [Open Questions](#open-questions)).

More info about the Kubernetes `EndpointSlices` resource and management can be found
in https://kubernetes.io/docs/concepts/services-networking/endpoint-slices/#management

Additional information can be added to the communication matrix to describe the use 
of the ports, for example traffic flows from master-to-master or from node-to-master
(an API change is required, could be indicated with annotations on the `Service`).

Periodic tests will be added to `openshift-tests` to validate an up-to-date 
generated communication matrix matches the communication matrix documented
in OpenShift release documents.

Another test will be added to validate the ports in a generated communication 
matrix match a snapshot of the node's listening ports (created with the Linux
`ss` utility).

The communication matrix module will be used to create and apply firewall rules, 
run E2E `openshift-tests` and upgrade the OpenShift version to validate the rules 
are acceptable by the current and next OCP release.

A user will be able to run `openshift-tests` or a new `oc` command, 
`oc adm communication-matrix validate`, to validate the `EndpointSlices` 
in the cluster match a current snapshot of the node's listening ports.

### Risks and Mitigations

1. Supporting egress traffic flows (out of scope) is more challenging than ingress as 
   there is no source of truth object such as the `EndpointSlices` for ingress traffic.

2. For egress flows support an API change might be needed, a change that can affect the 
   work done for ingress flows and should be reviewed and agreed with the upstream Kubernetes
   community.

3. Generating the communication matrix on MicroShift and/or in AdminNetworkPolicy format is 
   not supported yet in the current commatrix module.

### Drawbacks
N/A

## Open Questions

1. Is there a better OpenShift object to use than `EndpointSlices` to collect 
   information about ingress traffic?

2. Should a new OpenShift object be introduced to act as the source of truth
   for cluster traffic?

3. The node hosted services such as sshd, rpc, etc. are currently missing 
   `EndpointSlices`. We believe it should be created by the Machine API as 
    part of adding/removing nodes. This can be addressed in later enhancement.

4. Egress traffic support - how to effectively document the expected outgoing
   traffic from an OpenShift cluster?

5. Is there any additional information that should be added to the communication 
   matrix (for example the use of the ports)?

6. Currently OpenShift installer has its own hard-coded list of ports to open on
   different clouds. Can this eventually be auto-generated as well?

## Test Plan

- Unit tests coverage

- E2E tests will be added to `openshift-tests`
  - Validate an up-to-date generated communication matrix matches the 
    one documented in OpenShift release documents ([WIP PR](https://github.com/openshift/origin/pull/28946))
  - Validate the ports in a generated communication matrix match a snapshot
    of the node's listening ports (created with the Linux `ss` utility) ([WIP PR](https://github.com/openshift/origin/pull/28706))

- Release-informing/blocking jobs will be added
  - Use the communication matrix to create and apply firewall rules, and 
    run E2E `openshift-tests` ([implemented](https://github.com/openshift-kni/commatrix/blob/main/test/e2e/commatrix_suite_test.go#L100))
  - Use the communication matrix to create and apply firewall rules, and upgrade
    OpenShift version

- CI
  - An informing job running the communication matrix E2E tests will be set for a 
    period of time to validate stability before integrating to `openshift-tests`

## Graduation Criteria

### Dev Preview -> Tech Preview
N/A

### Tech Preview -> GA
GA: 4.16

### Removing a deprecated feature
N/A

## Upgrade / Downgrade Strategy
N/A

## Version Skew Strategy
N/A

## Operational Aspects of API Extensions
N/A

## Support Procedures
N/A

## Alternatives (Not Implemented)
N/A
