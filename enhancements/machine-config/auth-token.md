---
title: mco-auth-token
authors:
  - "@cgwalters"
reviewers:
  - "@crawford"
approvers:
  - "@ashcrow"
  - "@crawford"
  - "@imcleod"
  - "@runcom"
creation-date: 2020-08-05
last-updated: 2020-08-19
status: provisional
see-also:
replaces:
superseded-by:
---

# Support a provisioning token for the Machine Config Server

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement proposes that for new (e.g. 4.7+) installations a "provisioning token" is required by default
to access the Machine Config Server.

For IPI/machineAPI scenarios, this will be handled fully automatically.

For user-provisioned installations, openshift-install will generate a token internally via the equivalent of

```
$ dd if=/dev/urandom bs=32 count=1 | base64 > token
$ oc -n openshift-config create secret generic machineconfig-auth --from-file=token
```

And then this token will be injected into the "pointer ignition configs" it creates as a URL parameter, or
potentially `Authorization: bearer`.


## Motivation

The default Ignition configuration contains secrets (e.g. the pull secret, initial kubeconfig), and we want to avoid it being accessible both inside and outside the cluster.

### Goals

- Increase security
- Avoid breaking upgrades
- Easy to implement
- Easy to understand for operators
- Eventually support being replaced by something more sophisticated (e.g. TPM2 attestation)
- Avoid too strong dependence on platform-specific functionality (i.e. UPI AWS should remain close to  UPI bare metal)

### Non-Goals

- Completely rework major parts of the provisioning process today

## Proposal

Change the MachineConfigServer to support requiring a token in order to retrieve the Ignition config for new installs.

In IPI/machineAPI managed clusters, `openshift-install` and the MCO would automatically handle
this end to end; the initial implementation would work similarly to the UPI case, but may change
in the future to be even stronger.

### Implementation Details/Notes/Constraints

For UPI, the documentation would need to be enhanced to describe this.

For IPI, it's now possible to implement automatic rotation because the `user-data` provided to nodes is owned by the MCO: https://github.com/openshift/enhancements/blob/master/enhancements/machine-config/user-data-secret-managed.md

In order to avoid race conditions, the MCS might support the "previous" token for a limited period of time (e.g. one hour/day).

#### IPI/machineAPI details

It's probably easiest to start out by automatically generating a rotating secret that works the same way as UPI, and also (per above) support the previous token for a period of time.

### Risks and Mitigations

#### Troubleshooting

Debugging nodes failing in Ignition today is painful; usually doing so requires looking at the console.  There's no tooling or automation around this.  We will need to discuss this in a "node provisioning" section in the documentation.

This scenario would be easier to debug if [Ignition could report failures to the MCS](https://github.com/coreos/ignition/issues/585).

#### Disaster recovery

See: [Disaster recovery](https://docs.openshift.com/container-platform/4.5/backup_and_restore/disaster_recovery/about-disaster-recovery.html)

This proposes that the secret for provisioning a node is stored in the cluster itself (in the IPI case).  But so is the configuration for the cluster, so this is not a new problem.

Note that the token is only necessary when Ignition is run - which is the first boot of a node before it joins the cluster.  If a node is just shut down and restarted, access to the token isn't required.  Hence there are no concerns if e.g. the whole control plane (or full cluster) is shut down and restarted.

In the case of e.g. trying to reprovision a control plane node, doing that today already requires the Machine Config Server to run, which requires a cluster functioning enough to run it.  The auth token doesn't impose any additional burden to that.

That said, see "Rotating the token" below.

#### Non-Ignition components fetching the Ignition config

As part of the Ignition spec 3 transition, we needed to deal with non-Ignition consumers of the Ignition config, such as [openshift-ansible](github.com/openshift/openshift-ansible/) and Windows nodes.  These should generally instead fetch the rendered `MachineConfig` object from the cluster, and not access the MCS port via TCP.

#### Rotating the token

In UPI scenarios, we should document rotating the token, though doing so incurs some danger if the administrator forgets to update the pointer configuration.  We should recommend against administrators rotating the token *without* also implementing a "periodic reprovisioning" policy to validate that they can restore workers (and ideally control plane) machines.

Or better, migrate to an IPI/machineAPI managed cluster.

### Upgrades

This will not apply by default on upgrades, even in IPI/machineAPI managed clusters (to start).  However, an administrator absolutely could enable this "day 2".  Particularly for "static" user provisioned infrastructure where the new nodes are mostly manually provisioned, the burden of adding the token into the process of provisioning would likely be quite low.

For existing machineAPI managed installs, it should be possible to automatically adapt existing installs to use this, but we would need to very carefully test and roll out such a change; that would be done as a separate phase.

## Alternatives

#### Firewalling

Not obvious to system administrators in e.g. bare metal environments and difficult to enforce with 3rd party SDNs.

#### Move all secret data out of Ignition into the pointer config

Move the pull secret and bootstrap kubeconfig into the "pointer Ignition".  In all cloud environments that we care about, access to the "user data" in which the pointer Ignition is stored is secured.  It can't be accessed outside the cluster because it's a link local IP address, and the OpenShift SDN blocks it from the pod network (see references).

Risk: In UPI scenarios, particularly e.g. bare metal and vSphere type UPI that often involves a manually set up webserver, we do not currently require confidentiality for the pointer configuration.  This would need to change - it'd require documentation.

#### Encrypt secrets in Ignition

Similar to above, we have a bootstrapping problem around fetching the key.

#### Improved node identity

Docs for instance identity in GCP: https://cloud.google.com/compute/docs/instances/verifying-instance-identity
We may be able to do this in other clouds too.  [This blog post](https://googleprojectzero.blogspot.com/2020/10/enter-the-vault-auth-issues-hashicorp-vault.html) discusses a vulnerability in Vault but shows an abstraction over cloud identity systems.

In this model we'd change the Ignition process to require the node prove is identity, by passing a signed token or equivalent and the MCS would verify that.

Another approach on bare metal may be using [Trusted Platform Module](https://en.wikipedia.org/wiki/Trusted_Platform_Module)s for the whole provisioning process, including [CSR approval](https://github.com/openshift/cluster-machine-approver).

Also related: https://kubernetes.io/docs/reference/access-authn-authz/bootstrap-tokens/

