---
title: aws-ccm-cco-credentials
authors:
  - "@mfbonfigli"
reviewers:
  - "@JoelSpeed"
  - "@mtulio"
  - "@patrickdillon"
approvers:
  - "@mtulio"
  - "@patrickdillon"
api-approvers:
  - None
creation-date: 2026-07-30
last-updated: 2026-07-31
status: provisional
tracking-link:
  - https://issues.redhat.com/browse/SPLAT-2862
see-also:
  - "https://issues.redhat.com/browse/OCPBUGS-98763"
---

# AWS CCM: Migrate from IMDS Instance Role to CCO-Managed Credentials

## Summary

AWS Cloud Controller Manager (CCM) currently authenticates to AWS by reading credentials from the EC2 Instance Metadata Service (IMDS) via the master node's IAM instance role. This role is static from install time: there is no mechanism to extend it during cluster upgrades, causing new CCM features that require additional IAM permissions to fail on upgraded clusters. 

This enhancement proposes to migrate AWS CCM to the Cloud Credential Operator (CCO) model already used by other OpenShift cloud components. The Cluster Cloud Controller Manager Operator (CCCMO) declares the permissions CCM requires via a `CredentialsRequest` object and CCO satisfies it by creating and managing a credentials secret that the CCM pod mounts, bypassing IMDS entirely.

## Motivation

OpenShift installer creates a master node IAM instance role at install time. This role is static, upgrades have no mechanism to modify it and is accessible via IMDS to any container with host network access and any process running directly on the node OS, violating the least-privilege security best practice.

Additionally, when a new CCM feature requires a new IAM permission, that permission can only be granted by the installer. Clusters already installed with an older installer will never receive it automatically: the cluster admin is forced to manually apply the change to the master node IAM role, resulting in a poor customer experience inconsistent with how credentials are managed for the other cluster operators backed by CCO-provisioned credentials.

Finally, the breadth of CCM's IAM permissions forces the installer itself to hold elevated IAM privileges at install time. By moving CCM's permissions to a `CredentialsRequest` handled by CCO, the installer only needs to create a minimal master role, reducing its own required IAM footprint.

### User Stories

#### Story 1

As a cluster administrator running a Mint-mode cluster, I want CCM to automatically receive any new IAM permissions required by new releases without manual intervention, so that features relying on new permissions work on upgraded clusters without any action on my part.

#### Story 2

As a security-conscious cluster administrator, I want CCM's IAM permissions scoped to its pod rather than the master node instance role, so that no other process on the node inherits permissions it does not need.

#### Story 3

As a cluster administrator running a manual+STS mode cluster, I want to be able to manage CCM credentials via `ccoctl` as it is done for any other cluster component that requires AWS credentials via CCO, so to simplify the procedures during cluster upgrades and reduce the risk of errors.

### Goals

- CCM authenticates to AWS exclusively via CCO-managed credentials declared in a `CredentialsRequest`.
- The master node IAM instance role is reduced to only what is needed for master to operate in an AWS environment outside of CCM-specific permissions.
- No functional regression across all CCO modes: Mint, Passthrough, Manual, Manual+STS.
- Cluster upgrades from prior releases are seamless in Mint mode: CCO will automatically update the CCM permissions based on the new `CredentialRequest` definition.

### Non-Goals

- Migrating the kubelet credential provider away from IMDS.
- Removing the master node IAM instance role entirely.
- Modifying HyperShift / ROSA credential flows, which already operate differently and will be left untouched.
- Permission cleanup of the residual master node IAM role beyond the removal of CCM-related permissions.

## Proposal

### Workflow Description

#### Installing a new cluster

On a fresh install the flow is identical to every other CCO-managed operator:

1. **(Manual modes only)** If `credentialsMode` is set to `Manual`/`Manual+STS`/`Passthrough`, the administrator on cluster install/upgrade must verify and prepare credentials before installing the cluster according to the usual procedures for the respective mode. See the dedicated sections below for details.
2. CCCMO ships a `CredentialsRequest` manifest containing the IAM permissions AWS CCM requires to operate.
3. CVO applies the `CredentialsRequest` manifest from the CCCMO image.
4. CCO reconciles it and creates the `cloud-controller-manager-credentials` secret in `openshift-cloud-controller-manager`:
   - **Mint**: CCO mints a scoped IAM user with the declared permissions and puts the key pair into the secret.
   - **Passthrough**: CCO copies the root `aws-creds` into the secret.
   - **Manual (non-STS)**: CCO is a no-op. The admin must pre-create the `cloud-controller-manager-credentials` secret before CCCMO deploys the CCM Deployment, both during installation and upgrades.
   - **Manual+STS**: `ccoctl` (run pre-install) creates an OIDC-federated IAM role and pre-populates the secret with `role_arn` and `web_identity_token_file`.
5. CCCMO applies the updated CCM Deployment, which mounts the secret at `/etc/aws-credentials` and sets `AWS_SHARED_CREDENTIALS_FILE=/etc/aws-credentials/credentials`.
6. The AWS SDK inside CCM detects the environment variable and reads the credentials file.

The `AWS_SHARED_CREDENTIALS_FILE` variable automatically takes precedence over IMDS in the SDK credential chain, so IMDS is never reached. This means that no changes are needed in CCM for this approach to work, other than ensuring the variable is set and the credentials are mounted on the pod.

#### Manual CredentialsMode (IAM User / long-lived credentials)

In Manual mode CCO does not create or manage the credentials secret. The admin is
responsible for creating the `cloud-controller-manager-credentials` secret in the
`openshift-cloud-controller-manager` namespace before CCCMO deploys the updated CCM
Deployment.

The IAM user or role referenced by these credentials must have at minimum the permissions
listed in the `CredentialsRequest` `statementEntries`. If the secret is not present when
CCCMO applies the new Deployment, the CCM pod will remain `Pending` with a `FailedMount` event until the admin creates it.

On upgrades, if new `statementEntries` are added to the `CredentialsRequest` in a new
release, the admin must update the IAM user or role policy and, if the secret format
changes, update the secret itself before the new CCM pod can use the new permissions.

#### Manual CredentialsMode (STS / short-lived credentials)

For Manual+STS, a projected ServiceAccount token (audience: `sts.amazonaws.com`) is also mounted at `/var/run/secrets/openshift/serviceaccount/token`. The CCO-written credentials file contains:

```ini
[default]
role_arn = arn:aws:iam::<account>:role/<ccm-role>
web_identity_token_file = /var/run/secrets/openshift/serviceaccount/token
```

The AWS SDK reads the JWT, calls `sts:AssumeRoleWithWebIdentity`, and receives short-lived credentials. The kubelet rotates the projected token automatically before expiry. No custom rotation logic is needed in CCM.

The `cloudTokenPath` field in the `CredentialsRequest` tells CCO the exact path where the token will be mounted, so CCO writes the correct path into the credentials file.

### API Extensions

No API extensions.

### Topology Considerations

#### Hypershift / Hosted Control Planes

Not affected. In HyperShift HCP, CCCMO does not run in the guest cluster and CCM is deployed directly by the HyperShift `control-plane-operator` and uses a different system for managing CCM credentials with dedicated IAM Role, which is not being affected.

#### Standalone Clusters

Primary target. All CCO modes (Mint, Passthrough, Manual, Manual+STS) are supported.

#### Single-node Deployments or MicroShift

SNO uses the same CCCMO-managed CCM Deployment as multi-node clusters. The `CredentialsRequest` is applied identically. No unique considerations.

MicroShift does not use CCCMO or CCM. 

#### OpenShift Kubernetes Engine

No unique considerations.

### Implementation Details/Notes/Constraints

**No CCM binary changes required.** The AWS SDK's credential provider chain already supports `AWS_SHARED_CREDENTIALS_FILE` and the IRSA (`web_identity_token_file`) flow for
manual+STS mode. Setting the environment variable and mounting the credentials file is sufficient.

**Feature gate mechanism:** The entire change is gated behind the `AWSCCMCCOCredentials` OCP feature gate. The gate is evaluated at two points:

- **CCCMO:** At each reconcile loop, CCCMO reads the cluster feature gate. The result is passed as a boolean to the CCM Deployment template which uses `{{- if }}` blocks to conditionally include the `AWS_SHARED_CREDENTIALS_FILE` env var, the credentials secret volume mount, and the projected ServiceAccount token volume. When the gate is off, the rendered Deployment is identical to today's.
- **CredentialsRequest:** The CCM `CredentialsRequest` manifest carries the annotation `release.openshift.io/feature-gate: AWSCCMCCOCredentials`. If the feature gate is missing, the manifest will be ignored.
- **Installer:** The master role pruning is similarly conditioned on the feature gate at install time: when the gate is off the role retains the original broad permissions; when on it is created with `ec2:Describe*` only.

**Two repository changes:**

| Repository | Change |
| --- | --- |
| `openshift/cluster-cloud-controller-manager-operator` | Add `CredentialsRequest` manifest; add projected SA token volume, credentials secret volume, and `AWS_SHARED_CREDENTIALS_FILE` env var to the AWS CCM Deployment template |
| `openshift/installer` | Prune master IAM instance role to `ec2:Describe*` only, both the role created automatically during IPI installs and the CloudFormation template provided for UPI installs |

The installer change only affects new installs. Upgraded clusters retain their original master role (now unused by CCM) indefinitely.

**CredentialsRequest permission set:** The `CredentialsRequest` carries the permissions CCM actively uses, determined by analysis of the `cloud-provider-aws` codebase, the upstream prerequisites documentation, and the ROSA `ROSAKubeControllerPolicy`. 
EBS volume permissions (`ec2:AttachVolume`, `CreateVolume`, `DeleteVolume`, `DetachVolume`, `ModifyVolume`) present on the original master role are omitted as these operations are handled by the EBS CSI driver which uses a dedicated `CredentialsRequest` and thus are not needed in CCM.

```yaml
statementEntries:
- effect: Allow
  action:
  # Node lifecycle (read)
  - ec2:DescribeInstances
  - ec2:DescribeInstanceTopology
  - ec2:DescribeSecurityGroups
  - ec2:DescribeSubnets
  - ec2:DescribeAvailabilityZones
  - ec2:DescribeRouteTables
  - ec2:DescribeVpcs
  # Node lifecycle (write)
  - ec2:CreateSecurityGroup
  - ec2:CreateTags
  - ec2:DeleteSecurityGroup
  - ec2:AuthorizeSecurityGroupIngress
  - ec2:RevokeSecurityGroupIngress
  - ec2:ModifyInstanceAttribute
  - kms:DescribeKey
  # Load balancer management (CLB)
  - elasticloadbalancing:AddTags
  - elasticloadbalancing:AttachLoadBalancerToSubnets
  - elasticloadbalancing:ApplySecurityGroupsToLoadBalancer
  - elasticloadbalancing:CreateLoadBalancer
  - elasticloadbalancing:CreateLoadBalancerPolicy
  - elasticloadbalancing:CreateLoadBalancerListeners
  - elasticloadbalancing:ConfigureHealthCheck
  - elasticloadbalancing:DeleteLoadBalancer
  - elasticloadbalancing:DeleteLoadBalancerListeners
  - elasticloadbalancing:DeregisterInstancesFromLoadBalancer
  - elasticloadbalancing:DescribeLoadBalancers
  - elasticloadbalancing:DescribeLoadBalancerAttributes
  - elasticloadbalancing:DescribeLoadBalancerPolicies
  - elasticloadbalancing:DetachLoadBalancerFromSubnets
  - elasticloadbalancing:ModifyLoadBalancerAttributes
  - elasticloadbalancing:RegisterInstancesWithLoadBalancer
  - elasticloadbalancing:SetLoadBalancerPoliciesForBackendServer
  - elasticloadbalancing:SetLoadBalancerPoliciesOfListener
  # Load balancer management (NLB/ALB)
  - elasticloadbalancing:CreateListener
  - elasticloadbalancing:CreateTargetGroup
  - elasticloadbalancing:DeleteListener
  - elasticloadbalancing:DeleteTargetGroup
  - elasticloadbalancing:DescribeTargetGroups
  - elasticloadbalancing:DescribeTargetGroupAttributes
  - elasticloadbalancing:DescribeTargetHealth
  - elasticloadbalancing:DescribeListeners
  - elasticloadbalancing:DeregisterTargets
  - elasticloadbalancing:ModifyListener
  - elasticloadbalancing:ModifyTargetGroup
  - elasticloadbalancing:ModifyTargetGroupAttributes
  - elasticloadbalancing:RegisterTargets
  # BYO Security Group for NLB (OCPBUGS-98763)
  - elasticloadbalancing:SetSecurityGroups
  resource: "*"
```

### Companion installer change

A companion installer PR prunes the master node IAM instance role from ~45 actions to a single statement:

```json
{ "Effect": "Allow", "Action": ["ec2:Describe*"], "Resource": "*" }
```

This removes the permissions that CCM previously required from the instance role, ensuring they are now only accessible to the CCM pod via its scoped CCO credentials. No component other than CCM running on master nodes has been identified as requiring the removed permissions.

### Risks and Mitigations

| Risk | Mitigation |
|---|---|
| Upgraded cluster's master role still has the old fat permissions | CCM stops using these permissions once the credentials file is in place. Upgrade documentation will instruct the cluster admin to prune the role permissions down to `ec2:Describe*`. |
| CCM pod remains `Pending` (`FailedMount`) during upgrade while CCO creates the credentials secret | Expected and consistent behavior with all other CCO-managed operators. The pod starts automatically once CCO satisfies the `CredentialsRequest` and the secret becomes available. |
| Manual+STS admin forgets to run `ccoctl` on first upgrade to a CCO-managed release | The credentials secret is not created; CCM pod remains `Pending` (`FailedMount`). Upgrade documentation will provide the procedure to follow and troubleshooting steps. |
| Manual+STS admin forgets to run `ccoctl` before upgrading to a subsequent release with new permissions | CCM continues operating with the existing permission set. Features requiring the new permission produce `AccessDenied` error. Admin can remediate by updating the IAM role policy post-upgrade. |
| Installer and CCCMO changes land in different releases | If the installer PR (master role pruning) merges before the CCCMO PR, a window exists where the instance role is pruned but CCO credentials are not yet in place. Both PRs target the same release; the CCCMO PR must merge first. |
| Mint mode: long-lived IAM user key pair is compromised | The key pair remains valid until CCO revokes it. Mitigation: administrators with strict credential-lifetime requirements should use Manual+STS, which provides short-lived STS tokens via IRSA. |
| IAM UsersPerAccount quota impact | Each Mint-mode `CredentialsRequest` creates one IAM user. CCM adds one more to the existing set. This risk is pre-existing and not specific to this enhancement. |

### Drawbacks

- **Mint mode regresses credential lifetime.** Although this proposal improves the upgrade experience and moves toward least-privilege credential scoping, the existing IMDS-based approach provides short-lived credentials (AWS rotates instance role tokens automatically). Mint mode instead provides long-lived IAM user key pairs: if compromised, they remain valid until CCO revokes them rather than self-expiring. Only Manual+STS mode provides short-lived STS tokens that match or exceed the current security profile.

## Alternatives (Not Implemented)

**Keep permissions on master role** This is a no-op: do nothing and let CCM keep using the master node IAM role. This goes against the least-privilege principle and is a worse customer experience during cluster upgrades requiring new IAM permissions.

## Test Plan

**Unit tests** (`pkg/cloud/aws/aws_test.go`): assert the rendered Deployment contains `AWS_SHARED_CREDENTIALS_FILE`, both volume mounts, and both volumes.

**e2e tests — existing CI jobs that cover this change:**

| Scenario | CI job |
|---|---|
| New install, Mint mode — LB creation, node registration | `e2e-aws-ovn` |
| New install, Manual+STS — IRSA credentials, CCM operates | `periodic-ci-openshift-cloud-credential-operator-release-5.0-periodics-e2e-aws-manual-oidc` |
| Upgrade 4.22→5.x, Mint — CCO auto-updates IAM user policy | `periodic-ci-openshift-release-main-ci-5.0-upgrade-from-stable-4.22-e2e-aws-ovn-upgrade` |
| Hypershift regression | `periodic-ci-openshift-hypershift-release-5.0-periodics-e2e-aws-ovn-conformance-ccm` |

**Passthrough and Manual (non-STS) — manual validation:** No existing AWS CI job exercises these two CCO modes for any component. Secret creation/format, missing-Secret behavior, policy updates, and CCM AWS operations for these modes will be validated manually prior to GA.

## Graduation Criteria

This change is gated by the `AWSCCMCCOCredentials` feature gate.

### Dev Preview -> Tech Preview

N/A. This feature will be introduced as Tech Preview.

### Tech Preview -> GA

- E2E tests consistently passing with the feature gate enabled across all tested CCO modes.
- New IPI install and upgrade e2e from prior release passing.
- No regressions reported.
- All CCO modes validated (Mint, Passthrough, Manual, Manual+STS).
- User-facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/).

### Removing a deprecated feature

N/A.

## Upgrade / Downgrade Strategy

### Upgrade

In Mint mode, the upgrade will be fully automatic. CCO will provision new IAM user for CCM, and CCCMO will update CCM Deployment with the new configuration to consume it.

In Passthrough mode the Admin must manually ensure that the root credentials contain the permissions CCM needs prior to upgrading the cluster.

Manual+STS follows the same `ccoctl` re-run workflow as all other CCO-managed operators.

The master role on upgraded clusters retains its original permissions. CCM no longer uses them once the credentials file is in place, upgrade documentation will instruct the cluster admin to prune the role permissions down to `ec2:Describe*` as this cannot be done automatically.

### Downgrade

Downgrading removes the credentials secret and volume mounts from the Deployment spec. CCM falls back to IMDS and the master instance role, which on the downgraded cluster still has the original broad permissions, unless the cluster Admin removed them. If the admin removed IAM permissions from the master node IAM role, he needs to re-add them prior to downgrading the cluster.

## Version Skew Strategy

The `CredentialsRequest` is satisfied by CCO, which is a CVO-managed operator upgraded alongside CCM. No version skew risk exists between CCO and the `CredentialsRequest`
format.

The installer change only affects new installs. Upgraded clusters always retain their original master role, so a cluster running a new CCCMO against an old installer-created
master role is safe: CCM uses CCO credentials and ignores the master role entirely.

## Operational Aspects of API Extensions

No API extensions. N/A.

## Support Procedures

### Credentials secret missing after upgrade

CCO may not have reconciled yet. Check CCO health:

```sh
oc get co cloud-credential
oc logs -n openshift-cloud-credential-operator \
  deployment/cloud-credential-operator --tail=30 | \
  grep -i "error\|aws-cloud-controller"
```

### Verifying CCM is using CCO credentials

```sh
# Detect the mode from key names only — does not print credential values:
oc get secret -n openshift-cloud-controller-manager \
  cloud-controller-manager-credentials \
  -o jsonpath='{.data.credentials}' | base64 -d | grep '=' | cut -d '=' -f1
# Mint/Passthrough keys: aws_access_key_id, aws_secret_access_key
# Manual+STS keys:       role_arn, web_identity_token_file

# Master role should only have ec2:Describe* on new installs
INFRA_ID=$(oc get infrastructure cluster -o jsonpath='{.status.infrastructureName}')
aws iam get-role-policy \
  --role-name "${INFRA_ID}-master-role" \
  --policy-name "${INFRA_ID}-master-policy" \
  --query 'PolicyDocument.Statement[*].Action'
```

## Infrastructure Needed

None. All changes are in existing repositories with existing CI infrastructure.
