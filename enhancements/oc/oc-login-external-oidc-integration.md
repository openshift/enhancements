---
title: oc-login-external-oidc-integration
authors:
  - "@ardaguclu"
reviewers: 
  - "@deads2k"
  - "@stlaz"
approvers:
  - "@deads2k"
  - "@stlaz"
api-approvers:
  - "@deads2k"
creation-date: 2024-03-13
last-updated: 2024-03-13
tracking-link: 
  - https://issues.redhat.com/browse/WRKLDS-875
see-also:
  - "/enhancements/authentication/unsupported-direct-use-of-oidc.md"
  - https://kubernetes.io/docs/reference/access-authn-authz/authentication/#client-go-credential-plugins
---

# External OIDC Issuer Integration with oc login

## Summary

This enhancement proposal describes the mechanism for how users can log in to the cluster which relies on external OIDC issuer for authentication in lieu of
internal OAuth provided by OCP via `oc login`. In order to achieve this, this enhancement proposal adds new command, namely `get-token`, in oc that will serve
as the built-in credentials exec plugin and additionally OIDC specific flags in `oc login` to support this functionality.

## Motivation

Due to the gradual increase of the adoption of OCP by various customers, there is a proportional increase of demand to extend some of the functionalities in OCP.
Disabling internal OAuth server and relying on external OIDC issuers is one of them. Because some customers may have established their infrastructure 
on top of an external OIDC issuer so that they just want to reflect the same fine-grained principles into their OCP clusters as well. That's why, `oc login` should
have an enriched user interface to support both cases for a seamless usage.

### User Stories

#### Story 1

As a cluster administrator, I'd like to use the same roles/users in external OIDC issuer (e.g. Keycloak) in my OCP cluster.

### Goals

- Add support in `oc login` which will enable users to log in to their clusters if their cluster relies on external OIDC issuer, either by copying a log in command from webconsole, or manually.

### Non-Goals

- Change the oc login behavior against default internal OAuth functionality.

## Proposal

This enhancement document proposes a new oc command;

* `oc get-token`: Built-in credentials exec plugins of oc. It supports all the features that client-go requires to obtain the id token.
  * `--issuer-url`: Issuer URL of the external OIDC issuer and mandatory.
  * `--client-id`: Client ID that is created in external OIDC issuer and mandatory.
  * `--client-secret`: Optional field that can be passed, if cluster administrators intentionally choose to distribute this.
  * `--extra-scopes`: Includes all the extra scopes in a comma separated format (e.g. `--extra-scopes=email,profile`) and optional.
  * `--callback-address`: Callback address where external OIDC issuer redirects to after flow is completed. If it is not specified, command picks a random unused port.
  * `--auto-open-browser`: Specify browser is automatically opened or not. It is especially useful, if users want to use non-default browser.

and OIDC specific flags in `oc login`;
* `--exec-plugin`: Type indicates that which credentials exec plugin will be used by client-go. It only supports `oc-oidc` (uses `get-token` built-in command) currently, but can be easily extended.
* `--issuer-url`: Issuer URL of the external OIDC issuer and mandatory.
* `--client-id`: Client ID that is created in external OIDC issuer and mandatory.
* `--client-secret`: Optional field that can be passed, if cluster administrators intentionally choose to distribute this.
* `--extra-scopes`: Includes all the extra scopes in a comma separated format (e.g. `--extra-scopes=email,profile`) and optional.
* `--callback-port`: Callback port where external OIDC issuer redirects to after flow is completed. If it is not specified, command picks a random unused port and concatenating it to 127.0.0.1 

### Workflow Description

oc login generates a stanza in kubeconfig supporting credentials exec plugin feature in client-go.

For example;

```shell
$ oc login localhost:8443 ---issuer-url=https://oidc.issuer.url --exec-plugin=oc-oidc --client-id=client-id --extra-scopes=email,profile --callback-port=8080
```

results in;

```yaml
- name: localhost:8443
  user:
    exec:
      apiVersion: client.authentication.k8s.io/v1
      args:
      - get-token
      - --issuer-url=https://oidc.issuer.url
      - --client-id=client-id
      - --callback-address=127.0.0.1:8080
      - --extra-scopes=email,profile
      command: oc
      env: null
      installHint: Please be sure that oc is defined in $PATH to be executed as credentials
        exec plugin
      interactiveMode: IfAvailable
      provideClusterInfo: false
```

Moreover, `oc login` command triggers authentication process by sending a request to Project API's whoami endpoint automatically so that
client-go talks to `oc get-token` command to obtain id token (and also refresh token, if provider supports it).

There are currently 7 authentication flows grouped around public and confidential clients. Supportability (in terms of authentication flows) matrix of this feature is outlined below; 

|                                     | Authorization Code (public)                                                | Authorization Code (confidential)                                                                        | Authorization Code + PKCE (public)                                         | Authorization Code + PKCE (confidential)                                                                 | Implicit Flow (public) | Client Credentials (confidential)                         | Device Code (public)                                      | Password Grant (confidential) |
|-------------------------------------|----------------------------------------------------------------------------|----------------------------------------------------------------------------------------------------------|----------------------------------------------------------------------------|----------------------------------------------------------------------------------------------------------|------------------------|-----------------------------------------------------------|-----------------------------------------------------------|-------------------------------|
| Supported                           | Yes                                                                        | Yes                                                                                                      | Yes                                                                        | Yes                                                                                                      | No                     | No                                                        | No                                                        | No                            |
| Comment                             | Keycloak - [configurable] Azure Entra ID - [configurable]                  | Keycloak - [not-configurable] Azure Entra ID - [configurable]                                            | Keycloak - [configurable] Azure Entra ID - [configurable]                  | Keycloak - [not-configurable] Azure Entra ID - [configurable]                                            | Deprecated[1]          | Keycloak - [configurable] Azure Entra ID - [configurable] | Keycloak - [configurable] Azure Entra ID - [configurable] | Deprecated[2]                 |
| Command                             | `oc login api.url --issuer-url=external.oidc.issuer --client-id=client-id` | `oc login api.url --issuer-url=external.oidc.issuer --client-id=client-id --client-secret=client-secret` | `oc login api.url --issuer-url=external.oidc.issuer --client-id=client-id` | `oc login api.url --issuer-url=external.oidc.issuer --client-id=client-id --client-secret=client-secret` |                        |                                                           |                                                           |                               |
| Console Copy Login Command Supports | Yes                                                                        | No                                                                                                       | Yes                                                                        | No                                                                                                       | No                     | No                                                        | No                                                        | No                            |

[1] https://datatracker.ietf.org/doc/html/draft-ietf-oauth-v2-1-10#name-removal-of-the-oauth-20-imp

[2] https://datatracker.ietf.org/doc/html/draft-ietf-oauth-security-topics-24#section-2.4

Although it is also one of the authentication flows, refresh token is not added into this table above. Refresh token is provided by OIDC provider
during an authorization code flow and it is used by oidc plugin (i.e. `oc get-token`) to renew id token.

### API Extensions

N/A

### Topology Considerations

#### Hypershift / Hosted Control Planes

N/A

#### Standalone Clusters

N/A

#### Single-node Deployments or MicroShift

N/A

### Implementation Details/Notes/Constraints

### Risks and Mitigations

Additional configuration steps need to be performed in external OIDC side which may have slight negative impact on usability initially. 
For instance, an administrator must create an OIDC client within their identity provider so that it can be used by their users

### Drawbacks

There is no drawback because this feature has no relation with the current functionality.

## Open Questions [optional]


## Test Plan

Currently, test plan is mostly managed around unit test but in the future it is inevitable to have e2e test executed by our CI system.

## Graduation Criteria

N/A

### Dev Preview -> Tech Preview

N/A

### Tech Preview -> GA

N/A

### Removing a deprecated feature

N/A

## Upgrade / Downgrade Strategy

N/A

## Version Skew Strategy

Command will fail gracefully during authentication against older clusters or the clusters external OIDC is not enabled.

* If copied command from console is for external OIDC issuer and this command is run on old oc, oc will return unknown flag error.
* if kubeconfig is generated with new oc and new command execution is performed by old oc, client-go will not be able to find the `oc get-token` command and will return unknown command error.

## Operational Aspects of API Extensions

N/A

## Support Procedures

N/A

## Alternatives

We have decided to pursue implementing our own credentials exec plugin embedded in oc. But there are other options that
were considered as alternatives.

### [int128/kubelogin](https://github.com/int128/kubelogin)

This plugin is well-known by the community and it is widely used. Furthermore, repository seems to be actively maintained and
security patches are performed regularly. However, this would lead us to forcing users to download additional binary in addition to oc
and copying login command from console may not work seamlessly in case where users don't install this plugin. 

Supportability (in terms of authentication flows) matrix of this plugin is outlined below;

|           | Authorization Code (public) | Authorization Code (confidential) | Authorization Code + PKCE (public) | Authorization Code + PKCE (confidential) | Implicit Flow (public) | Client Credentials (confidential)                                    | Refresh Token (public/confidential) | Device Code (public) | Password Grant (confidential) |
|-----------|-----------------------------|-----------------------------------|------------------------------------|------------------------------------------|------------------------|----------------------------------------------------------------------|-------------------------------------|----------------------|-------------------------------|
| Supported | Yes                         | Yes                               | Yes                                | Yes                                      | No                     | No                                                                   | Yes                                 | Yes                  | Yes                           |
| Comment   |                             | Only if provider supports this    |                                    | Only if provider supports this[1]        | Deprecated             | Mostly used in CI systems that  client-secret can be stored securely | If provider supports it             |                      | Deprecated                    |

### [stiants/keycloak-oidc-cli](https://github.com/stianst/keycloak-oidc-cli)

This plugin is maintained by Keycloak team at Red Hat. However, this also leads us to forcing users to download additional binary in addition to oc.

Supportability (in terms of authentication flows) matrix of this plugin is outlined below;

|           | Authorization Code (public) | Authorization Code (confidential) | Authorization Code + PKCE (public) | Authorization Code + PKCE (confidential) | Implicit Flow (public) | Client Credentials (confidential)                                    | Refresh Token (public/confidential) | Device Code (public) | Password Grant (confidential) |
|-----------|-----------------------------|-----------------------------------|------------------------------------|------------------------------------------|------------------------|----------------------------------------------------------------------|-------------------------------------|----------------------|-------------------------------|
| Supported | No                          | No                                | Yes                                | No                                       | No                     | Yes                                                                  | Yes                                 | Yes                  | Yes                           |
| Comment   |                             |                                   |                                    |                                          | Deprecated             | Mostly used in CI systems that  client-secret can be stored securely | If provider supports it             |                      | Deprecated                    |

### [azure/kubelogin](https://github.com/Azure/kubelogin)

This plugin is recommended by Azure (and also maintained by) to customers using Azure Entra ID and have several features specific to Azure Entra ID.
However, we are trying to provide a standard mechanism in oc login and besides this plugin also requires to be installed and managed separately.

I'd like to emphasize that in case customers request to use this plugin in oc login, `--exec-plugin` flag can be enriched to support this plugin easily.

Supportability (in terms of authentication flows) matrix of this plugin is outlined below;

|           | Authorization Code (public) | Authorization Code (confidential) | Authorization Code + PKCE (public) | Authorization Code + PKCE (confidential) | Implicit Flow (public) | Client Credentials (confidential)                                    | Refresh Token (public/confidential) | Device Code (public) | Password Grant (confidential) |
|-----------|-----------------------------|-----------------------------------|------------------------------------|------------------------------------------|------------------------|----------------------------------------------------------------------|-------------------------------------|----------------------|-------------------------------|
| Supported | No                          | No                                | Yes                                | Yes                                      | No                     | Yes                                                                  | Yes                                 | Yes                  | Yes                           |
| Comment   |                             |                                   |                                    | Only if provider supports this           | Deprecated             | Mostly used in CI systems that  client-secret can be stored securely | If provider supports it             |                      | Deprecated                    |


## Infrastructure Needed [optional]

N/A
