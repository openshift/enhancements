---
title: azure-workload-identity
authors:
  - abutcher
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - TBD
approvers:
  - TBD
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - TBD
creation-date: yyyy-mm-dd
last-updated: yyyy-mm-dd
tracking-link:
  - https://issues.redhat.com/browse/CCO-187
see-also:
  - "enhancements/cloud-integration/aws/aws-pod-identity.md"
replaces:
  - ""
superseded-by:
  - ""
---

# Azure Workload Identity

## Summary

Core OpenShift operators (e.g. ingress, image-registry, machine-api) use long-lived credentials to access Azure API services today. This enhancement proposes an implementation by which OpenShift operators would utilize short-lived, [bound service account tokens](https://docs.openshift.com/container-platform/4.11/authentication/bound-service-account-tokens.html) signed by OpenShift that can be
trusted by Azure as the `ServiceAccounts` have been associated with [Azure Managed Identities](https://learn.microsoft.com/en-us/azure/active-directory/managed-identities-azure-resources/overview). [Workload identity federation support for Managed Identities](https://github.com/Azure/azure-workload-identity/issues/325) was recently made public preview by Azure
([announcement](https://learn.microsoft.com/en-us/azure/aks/workload-identity-overview)) and is the basis for this proposal.

## Motivation

Previous enhancements have implemented short-lived credential support via [STS for AWS](https://github.com/openshift/enhancements/pull/260) and GCP Workload Identity. This enhancement proposal intends to compliment those implementations within the Azure platform.

### User Stories

- As a cluster-creator, I want to create a self-managed OpenShift cluster on Azure that utilizes short-lived credentials for core operator authentication to Azure API services so that long-lived credentials do not live on the cluster.
- As a cluster-administrator, I want to provision Managed Identities within Azure and use those for my own workload's authentication to Azure API services.

### Goals

- Core OpenShift operators utilize short-lived, bound service account token credentials to authenticate with Azure API Services.
- Self-managed OpenShift administrators can create Azure Managed Identities via `ccotcl`'s processing of `CredentialsRequest` custom resources extracted from the release image prior to installation and provide the secrets output as manifests for installation which serve as the credentials for core OpenShift operators.
- An admin can create an Azure Managed Identity and Federated Credential via `CredentialRequest` CR and inject (via annotation) to a `ServiceAccount`, just as they can create an Azure service principal and inject to a `Secret` currently.

### Non-Goals

- Creation of Azure Managed Identity infrastructure (OIDC, managed identities, federated credentials) in managed environments (eg. ARO).
- Role granularity for the explicit necessary permissions granted to Managed Identities.

## Proposal

In this proposal, the Cloud Credential Operator's command-line utility (`ccoctl`) will be extended with subcommands for Azure which will provide methods for generating the manifests necessary to create an Azure cluster that utilizes Azure Workload Identity for core OpenShift operator authentication.

OpenShift operators will be updated to create Azure clients using the operator's bound `ServiceAccount` token that has been associated with the `clientID` of a Managed Identity in Azure.

Managed Identity details such as the `clientID` and `tenantID` necessary for creating a client can also be supplied to pods as environment variables via a [mutating admission webhook provided by Azure Workload Identity](https://azure.github.io/azure-workload-identity/docs/installation/mutating-admission-webhook.html). This webhook would be deployed and lifecycled by the Cloud Credential Operator
such that it could be utilized to supply credential details to customer workloads.

### Workflow Description

#### Cloud Credential Operator Command-line Utility (ccoctl)

The Cloud Credential Operator's command-line utility (`ccoctl`) will be extended with subcommands for Azure which provide methods for,
- Generating a key pair to be used for `ServiceAccount` token signing for a fresh OpenShift cluster.
- Creating an Azure blob storage container to serve as the identity provider in which to publish OIDC and JWKS documents needed to establish trust at a publically available address. This subcommand will output a modified cluster `Authentication` CR, containing a `serviceAccountIssuer` pointing to the Azure blob storage container's URL to be provided as a manifest for installation.
- Creating Managed Identity infrastructure with federated credentials for OpenShift operator `ServiceAccounts` (identified by namespace & name) and to output secrets containing the `clientID` of the Managed Identity to be provided as manifests for the installer. This command will process `CredentialsRequest` custom resources to identify service accounts that will be associated with Managed
  Identities in Azure as federated credentials. For self-managed installation, `CredentialsRequests` will be exracted from the release image.

```sh
$ ./ccoctl azure
Creating/updating/deleting cloud credentials objects for Azure

Usage:
  ccoctl azure [command]

Available Commands:
  create-all                Create key pair, identity provider and Azure Managed Identities
  create-identity-provider  Create identity provider
  create-managed-identities Create Azure Managed Identities
  create-key-pair           Create a key pair
  delete                    Delete Azure identity provider and Managed Identity infrastructure

Flags:
  -h, --help   help for azure

Use "ccoctl azure [command] --help" for more information about a command.
```

#### Credentials secret

OpenShift operators currently obtain their long-lived credentials from a config secret with the following format:

```yaml
apiVersion: v1
data:
  azure_client_id: <client id>
  azure_client_secret: <client secret>
  azure_region: <region>
  azure_resource_prefix: <resource group prefix eg. "abutcher-az-t68n4">
  azure_resourcegroup: <resource group eg. "abutcher-az-t68n4-rg">
  azure_subscription_id: <subscription id>
  azure_tenant_id: <tenant id>
kind: Secret
type: Opaque
```

We propose that when utilizing Azure Workload Identity, the credentials secret will contain an `azure_client_id` that is the `clientID` of the Managed Identity provisioned by `ccotcl` for the operator. The `azure_client_secret` key will be absent and instead we can provide the path to the mounted `ServiceAccount` token as an `azure_federated_token_file` key; the path to the mounted token is well
known and is specified in the operator deployment.

The resource group in which the installer will create infrastructure will not be known when these secrets are generated by `ccoctl` ahead of installation and operators which rely on `azure_resourcegroup` and `azure_resource_prefix` such as the
[image-registry](https://github.com/openshift/cluster-image-registry-operator/blob/8556fd48027f89e19daad36e280b60eb93d012d4/pkg/storage/azure/azure.go#L95-L100) should obtain the resource group details from the cluster `Infrastructure` object instead.

```yaml
apiVersion: v1
data:
  azure_client_id: <client id>
  azure_federated_token_file: <path to mounted service account token, eg. "/var/run/secrets/openshift/serviceaccount/token">
  azure_region: <region>
  azure_subscription_id: <subscription id>
  azure_tenant_id: <tenant id>
kind: Secret
type: Opaque
```

#### Creating workload identity clients in operators

In order to create Azure clients which utilize a `ClientAssertionCredential`, operators must update to version `>= v1.2.0` of the azidentity package within [azure-sdk-for-go](https://pkg.go.dev/github.com/Azure/azure-sdk-for-go/sdk/azidentity@v1.2.0). Ahead of this work, due to the [end of life
announcement](https://techcommunity.microsoft.com/t5/microsoft-entra-azure-ad-blog/microsoft-entra-change-announcements-september-2022-train/ba-p/2967454) of the Azure Active Directory Authentication Library (ADAL), PRs (ex. [openshift/cluster-ingress-operator](https://github.com/openshift/cluster-ingress-operator/pull/846)) have been opened for operators to migrate to creating clients via
azidentity which are converted into an authorizer for use with v1 clients. Once these changes have been made, we propose that OpenShift operators continue to utilize a config secret to obtain authentication details as described in the previous section but create workload identity clients when the `azure_client_secret` is absent AND/OR when  `azure_federated_token_file` fields are found in the
config. Config secrets will be generated by cluster creators prior to installation by using `ccoctl` and will be provided as manifests for install.

Due to the deployment of the Azure Workload Identity mutating admission webhook, environment variables should also be respected by client instatiation as an alternative way of supplying the `clientID` eg. `AZURE_CLIENT_ID`, `tenantID` eg. `AZURE_TENANT_ID` and `federatedTokenFile` eg. `AZURE_FEDERATED_TOKEN_FILE`.

Code sample ([commit](https://github.com/openshift/cluster-ingress-operator/commit/0461800fdcc5a67524e4bbfe0da2db551b0437be
)) taken from a [proof of concept](https://gist.github.com/abutcher/2a92d678a6da98d5c98a188aededab69) based on [openshift/cluster-ingress-operator](https://github.com/openshift/cluster-ingress-operator/pull/846):

All operators would need code changes similar to the sample below.

```go
type workloadIdentityCredential struct {
	assertion, file string
	cred            *azidentity.ClientAssertionCredential
	lastRead        time.Time
}

type workloadIdentityCredentialOptions struct {
	azcore.ClientOptions
}

func newWorkloadIdentityCredential(tenantID, clientID, file string, options *workloadIdentityCredentialOptions) (*workloadIdentityCredential, error) {
	w := &workloadIdentityCredential{file: file}
	cred, err := azidentity.NewClientAssertionCredential(tenantID, clientID, w.getAssertion, &azidentity.ClientAssertionCredentialOptions{ClientOptions: options.ClientOptions})
	if err != nil {
		return nil, err
	}
	w.cred = cred
	return w, nil
}

func (w *workloadIdentityCredential) GetToken(ctx context.Context, opts policy.TokenRequestOptions) (azcore.AccessToken, error) {
	return w.cred.GetToken(ctx, opts)
}

func (w *workloadIdentityCredential) getAssertion(context.Context) (string, error) {
	if now := time.Now(); w.lastRead.Add(5 * time.Minute).Before(now) {
		content, err := os.ReadFile(w.file)
		if err != nil {
			return "", err
		}
		w.assertion = string(content)
		w.lastRead = now
	}
	return w.assertion, nil
}

func getAuthorizerForResource(config Config) (autorest.Authorizer, error) {
  ...

	var (
		cred azcore.TokenCredential
		err  error
	)

  // ClientSecret is absent AND FederatedTokenFile has been set, create a workloadIdentityCredential
	if config.ClientSecret == "" && config.FederatedTokenFile != "" {
		options := workloadIdentityCredentialOptions{
			ClientOptions: azcore.ClientOptions{
				Cloud: cloudConfig,
			},
		}
		cred, err = newWorkloadIdentityCredential(config.TenantID, config.ClientID, config.FederatedTokenFile, &options)
		if err != nil {
			return nil, err
		}
	} else {
		options := azidentity.ClientSecretCredentialOptions{
			ClientOptions: azcore.ClientOptions{
				Cloud: cloudConfig,
			},
		}
		cred, err = azidentity.NewClientSecretCredential(config.TenantID, config.ClientID, config.ClientSecret, &options)
		if err != nil {
			return nil, err
		}
	}

	scope := config.Environment.TokenAudience
	if !strings.HasSuffix(scope, "/.default") {
		scope += "/.default"
	}
	// Use an adapter so azidentity in the Azure SDK can be used as
	// Authorizer when calling the Azure Management Packages, which we
	// currently use. Once the Azure SDK clients (found in /sdk) move to
	// stable, we can update our clients and they will be able to use the
	// creds directly without the authorizer. The schedule is here:
	// https://azure.github.io/azure-sdk/releases/latest/index.html#go
	authorizer := azidext.NewTokenCredentialAdapter(cred, []string{scope})
	return authorizer, nil
}
```

#### Mutating admission webhook

CCO will deploy and lifecycle the [Azure Workload Identity mutating admission webhook](https://azure.github.io/azure-workload-identity/docs/installation/mutating-admission-webhook.html) on Azure clusters such that customers can annotate workload `ServiceAccounts` with Managed Identity details necessary for creating clients. When the mutating admission webhook finds these annotations on a
`ServiceAccount` referenced by a pod being created, environment variables are set for the pod for the `AZURE_CLIENT_ID`, `AZURE_TENANT_ID` and `AZURE_FEDERATED_TOKEN_FILE`.

This will be similar to how CCO deploys the [AWS Pod Identity webhook](https://github.com/openshift/aws-pod-identity-webhook) which we have forked for use by customer workloads.

#### Variation [optional]

TBD

### API Extensions

None as of now.

### Implementation Details/Notes/Constraints [optional]

TBD

### Risks and Mitigations

- The feature this work relies on was recently made public preview. What is the timeline for GA for Workload identity federation support for Managed Identities?
- How will security be reviewed and by whom?
- How will UX be reviewed and by whom?

### Drawbacks

The pod identity webhook deployed for AWS has received little ongoing maintenance since its initial deployment by CCO and this proposal adds yet another webhook to by lifecycled by CCO, however upstream seems to be moving in this direction for providing client details as opposed to config secrets. It is likely best for compatibility with how operators currently obtain client information from a
config secret while also respecting the environment variables that would be set by the webhook. Additionally, upstream projects may reject the notion of reading these details from a config secret but that has yet to be seen.

## Design Details

### Open Questions [optional]

- From where should CCO source the mutating admission webhook for deployment?

### Test Plan

An e2e test job will be created similar to the [e2e-gcp-manual-oidc](https://github.com/openshift/release/pull/22552) that,
- Extracts `CredentialsRequests` from the release image.
- Processes `CredentialsRequests` with `ccoctl` to generate secret and `Authentication` configuration manifests.
- Moves the generated manifests into the manifests directory used for install.
- Runs the normal e2e suite against the resultant cluster.

### Graduation Criteria

#### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers
- Enumerate service level indicators (SLIs), expose SLIs as metrics
- Write symptoms-based alerts for the component(s)

#### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default
- Backhaul SLI telemetry
- Document SLOs for the component
- Conduct load testing
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

#### Removing a deprecated feature

### Upgrade / Downgrade Strategy

As clusters are upgraded, new permissions may be required or extended (in the case of future role granularity work) and customers must evaluate those changes at the upgrade boundary similarly to [upgrading an STS cluster in manual mode](https://docs.openshift.com/container-platform/4.11/authentication/managing_cloud_provider_credentials/cco-mode-manual.html#manual-mode-sts-blurb).

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

### Operational Aspects of API Extensions

Describe the impact of API extensions (mentioned in the proposal section, i.e. CRDs,
admission and conversion webhooks, aggregated API servers, finalizers) here in detail,
especially how they impact the OCP system architecture and operational aspects.

- For conversion/admission webhooks and aggregated apiservers: what are the SLIs (Service Level
  Indicators) an administrator or support can use to determine the health of the API extensions

  Examples (metrics, alerts, operator conditions)
  - authentication-operator condition `APIServerDegraded=False`
  - authentication-operator condition `APIServerAvailable=True`
  - openshift-authentication/oauth-apiserver deployment and pods health

- What impact do these API extensions have on existing SLIs (e.g. scalability, API throughput,
  API availability)

  Examples:
  - Adds 1s to every pod update in the system, slowing down pod scheduling by 5s on average.
  - Fails creation of ConfigMap in the system when the webhook is not available.
  - Adds a dependency on the SDN service network for all resources, risking API availability in case
    of SDN issues.
  - Expected use-cases require less than 1000 instances of the CRD, not impacting
    general API throughput.

- How is the impact on existing SLIs to be measured and when (e.g. every release by QE, or
  automatically in CI) and by whom (e.g. perf team; name the responsible person and let them review
  this enhancement)

#### Failure Modes

- Describe the possible failure modes of the API extensions.
- Describe how a failure or behaviour of the extension will impact the overall cluster health
  (e.g. which kube-controller-manager functionality will stop working), especially regarding
  stability, availability, performance and security.
- Describe which OCP teams are likely to be called upon in case of escalation with one of the failure modes
  and add them as reviewers to this enhancement.

#### Support Procedures

Describe how to
- detect the failure modes in a support situation, describe possible symptoms (events, metrics,
  alerts, which log output in which component)

  Examples:
  - If the webhook is not running, kube-apiserver logs will show errors like "failed to call admission webhook xyz".
  - Operator X will degrade with message "Failed to launch webhook server" and reason "WehhookServerFailed".
  - The metric `webhook_admission_duration_seconds("openpolicyagent-admission", "mutating", "put", "false")`
    will show >1s latency and alert `WebhookAdmissionLatencyHigh` will fire.

- disable the API extension (e.g. remove MutatingWebhookConfiguration `xyz`, remove APIService `foo`)

  - What consequences does it have on the cluster health?

    Examples:
    - Garbage collection in kube-controller-manager will stop working.
    - Quota will be wrongly computed.
    - Disabling/removing the CRD is not possible without removing the CR instances. Customer will lose data.
      Disabling the conversion webhook will break garbage collection.

  - What consequences does it have on existing, running workloads?

    Examples:
    - New namespaces won't get the finalizer "xyz" and hence might leak resource X
      when deleted.
    - SDN pod-to-pod routing will stop updating, potentially breaking pod-to-pod
      communication after some minutes.

  - What consequences does it have for newly created workloads?

    Examples:
    - New pods in namespace with Istio support will not get sidecars injected, breaking
      their networking.

- Does functionality fail gracefully and will work resume when re-enabled without risking
  consistency?

  Examples:
  - The mutating admission webhook "xyz" has FailPolicy=Ignore and hence
    will not block the creation or updates on objects when it fails. When the
    webhook comes back online, there is a controller reconciling all objects, applying
    labels that were not applied during admission webhook downtime.
  - Namespaces deletion will not delete all objects in etcd, leading to zombie
    objects when another namespace with the same name is created.

## Implementation History

## Alternatives

## Infrastructure Needed [optional]

