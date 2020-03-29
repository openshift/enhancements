---
title: openshift-console-cloud-shell
authors:
  - "@l0rd"
  - "@davidfestal"
reviewers:
  - "@deads2k"
  - "@christianvogt"
  - "@serenamarie125"
  - "@sspeiche"
  - "@sleshchenko"
  - "@AndrienkoAleksandr"
  - TBD
approvers:
  - "@deads2k"
  - TBD
creation-date: 2020-01-20
last-updated: 2020-01-20
status: provisional
---

# OpenShift Console Cloud Shell

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Open Questions [optional]

- [x] Initial "exec" denial mechanism PoC (with 1 validating webhook only)
- [x] Complete "exec" denial mechanism PoC using [che-workspace-crd-operator](https://github.com/che-incubator/che-workspace-crd-operator) (with 1 mutating + 3 validating webhooks). See section "Pod Exec Denial Policy" below.
- [ ] Define the mechanism to inject user tokens in the tooling container. See section "Users Token Injection in Tooling Containers" below.
- [ ] Analyse if [Kubernetes RBAC](https://github.com/brancz/kube-rbac-proxy) works better for us then [OpenShift OAuth Proxy](https://github.com/openshift/oauth-proxy) as the auth proxy sidecar.

## Summary

The following enhancement proposal is about a secured web based terminal embedded in the OpenShift console. It will be based on [Eclipse Che workspaces](https://github.com/eclipse/che) web terminal and will be available for any OpenShift authenticated user.

![image](https://user-images.githubusercontent.com/606959/70440704-100ca400-1a93-11ea-9a29-3af92d321d12.png)

## Motivation

A shell embedded in the OpenShift console will allow user to use OpenShift command line (`oc`) without installing tools locally and without the need to view copy and paste their auth token.

### Goals

- Add link on the OpenShift Console that provisions a dedicated user terminal
- Inject users identity in their terminal containers
- Provide basic command line tooling: `bash`, `oc`, `odo`, `jq`, `vim`
- Users that have cluster wide edit or admin roles cannot "exec" into the terminal containers and in any case retrieve users tokens

### Non-Goals

- Provide the ability to install tools and applications in the terminal container
- Persist user `/home` folder amongst sessions
- Store users tokens in secrets
- Mount users tokens as persistent volumes

## Proposal

### Background - Che Workspaces Web Terminal

We currently embed a web terminal in Che workspaces:

![](https://i.imgur.com/N3Bs1iS.png)


It uses [xterm.js](https://xtermjs.org) client side and, a custom "terminal server" written in go, server side. The backend server exposes a websocket stream to which xterm.js will [attach to](https://xtermjs.org/docs/api/addons/attach/). The backend server does an `exec` to start a terminal in another container and attach it to xterm.js.

It is secured using a JWT proxy sidecar that verifies that the user is authorized to access the terminal. A user is only authorized to access his workspaces terminals.

### Implementation details

Eclipse Che workspaces already provide a web based secured shell. The idea is to reuse part of a Che workpace components to implement the OpenShift Console Cloud Shell. In the following sections we are going into the details of:
- How does a Cloud Shell instance compares to an Eclipse Che workspace
- Authentication and authorization mechanism to secure CloudShell instances
- The mechanism to deny any user other then the owner to `exec` into a Cloud Shell Pod

#### Cloud Shell vs Che Workspace

A cloud shell instance will be implemented as a minimal Che workspace pod that includes the following 3 containers:

- [OpenShift OAuth Proxy](https://github.com/openshift/oauth-proxy): for authentication
- Terminal server: connects to `xterm.js` and does `exec` to spawn shells in the tooling container 
- Tooling container: with CLI tools like `bash`, `oc`, `odo`, `jq`, `vim` and stuff

![](https://i.imgur.com/3mSAe1w.png)


Such a workspace will be defined using a `DevWorkspace` Custom Resource (c.f. [#15425](https://github.com/eclipse/che/issues/15425)):

```yaml
apiVersion: workspaces.ecd.eclipse.org/v1alpha1
kind: DevWorkspace
metadata:
  name: cloudshell
spec:
  components:
    - type: dockerimage
      id: eclipse/cloudshell/latest
``` 

As a consequence, the Cloud Shell will be dependent on the Eclipse Che DevWorkspace operator that deploys the CRD and controller to manage workspaces. But unlike Eclipse Che, in a Cloud Shell instance:
- Eclipse Che server components are not deployed: `wsmaster`, `registries`, `keycloak` and `postgres`. Instanciation of `DevWorkspaces` is a responsibility of the OpenShift Console.
- Eclipse Che and the Cloud Shell are both deployed and controlled using operators but those are distinct ones.
- Theia (or other editors) based workspaces are not supported. Only Eclipse Che workspaces with editor of type CloudShell are supported.

#### Authentication and Authorization

The Terminal server endpoint is exposed through a TLS route. That's the endpoint that the clients running on the browser side (xterm.js) will connect to.

![](https://i.imgur.com/XWcgHeI.png)

The endpoint is secured through an [OpenShift OAuth Proxy](https://github.com/openshift/oauth-proxy) sidecar. The client includes the user OpenShift token in its requests HTTP header. Only authenticated OpenShift users that satisfy the RBAC policy requirements are able to send requests to the Terminal server.

In order to prevent unauthorized request to the Terminal server (coming from inside the OpenShift cluster) a [Network Policy](https://kubernetes.io/docs/concepts/services-networking/network-policies/) is used.

#### Cloud Shell Pod Exec Denial Policy

Users tokens are injected in the Tooling container. That's required for `oc` or `odo` to work properly. As a consequence access to the Tooling container should be allowed **only to the user of the Cloud Shell instace**. That's something that the authorization mechanism based on RBAC seen above is not able to garantee: users with editor privileges at cluster scope will be able to exec into the Pod.

To prevent exec from users other then the Cloud Shell owner a [ValidatingAdmissionWebhook](https://kubernetes.io/docs/reference/access-authn-authz/admission-controllers/#validatingadmissionwebhook) is used.

![](https://i.imgur.com/gIqTnqP.png)

The Validating Webhook mechanism described above compares the user in the `AdmissionReview` request with the user that has requested the creation of the DevWorkspace CR associated to the Cloud Shell Pod. If users match the request is allowed, otherwise it's refused.

This Validating Webhook mechanism requires other dynamic admission controllers to work properly:

- A Mutating Admission Webhook that adds a user attribute to the DevWorkspace CR at creation time
- A Validating Admission Webhook that block changes to the user attribute of a DevWorkspace CR
- A Validating Admission Webhook that block changes to the Cloud Shell Pod annotation that stores the reference to the DevWorkspace CR.

#### Users Token Injection in Tooling Containers

That's something that we still need to figure out (see "Open Questions" section) but the injection of the user token cannot be achieved through a secret because cluster editors would be able to access it. Options are 1) xtermjs that automatically execute a command at startup 2) the `DevWorkspaces` controller that copies the token in the tooling container after it has been started.

#### CloudShell Lifecycle

1. **Deployment**
The DevWorkspace Operator deploys the DevWorkspace controller and CRD. It shoud be a dependency of the OpenShfit Console operator.

2. **Creation of a CloudShell instance**
When the user opens the terminal widget in the OpenShift Console, the UI will call the k8s API to create a DevWorkspace CR on behalf of the user (using his token). The controller will then be responsible to create the cloudshell pod and start the terminal server within it.

3. **Attaching the terminal to the terminal server**
Once the pod is ready `xtermjs` will be attached to it. Attach succeed only if the user is authenticated and authorized to.
The terminal service will run in a special container and will be requested to do the `exec`. The user token will be used to `exec` in the target container. No users other then the owner will be able to do the exec.

![](https://i.imgur.com/1J5qIjU.png)

4. **Logging in the user**
Once xterm.js has successfully started a remote session the user token is injected.

5. **Autoscale to zero**
After a period of inactivity the cloud shell pod is scaled to zero. The Terminal server measures the inactivity period and delete the DevWorkspace CR if the period exceed a given tiemout value. The DevWorkspace controller does the cleanup of the CloudShell resources.

## Design Details

### Test Plan

**Note:** *Section not required until targeted at a release.*


### Graduation Criteria

**Note:** *Section not required until targeted at a release.*


### Version Skew Strategy


## Implementation History


## Drawbacks


## Alternatives

### No exec

Current approach is to allow `exec` into the tooling container only to the owner of the `DevWorkspace` object. To setup this hardening mechanism we need to deploy some one mutating admission webhook and three validating admission webhooks.

An alternative may be to deny every `exec`, even for the owner of the `DevWorkspace`. That could be implemented with one single admission plugin making the design simpler. The drawback would be that the terminal service should be pre-packaged in the tooling container making the architecture less flexible.

![](https://i.imgur.com/s1imdyX.png)

### Secure the namespace

Current approach is to deny `exec` into the Cloud Shell pods using validating webhooks. An alternative to deny `exec` operations could be to avoid editors in the namespace where the Cloud Shell pods are deployed. The Cloud Shell pods namespace could be the console namespace and no user or SA would have edit rights in that namespace.
