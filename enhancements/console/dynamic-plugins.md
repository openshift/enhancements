---
title: dynamic-plugins
authors:
  - "@spadgett"
reviewers:
  - "@bparees"
  - "@shawn-hurley"
  - "@christianvogt"
  - "@vojtechszocs"
approvers:
  - "@bparees"
creation-date: 2020-08-18
last-updated: 2020-09-01
status: implementable
---

# Dynamic Plugins for OpenShift Console

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [x] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

OpenShift Console currently has
[static plugins](https://github.com/openshift/console/tree/master/frontend/packages/console-plugin-sdk)
that allow teams to contribute features to the UI such as CNV and OCS. These plugins live in a
[packages directory](https://github.com/openshift/console/tree/master/frontend/packages)
inside the `openshift/console` repo and extend console through a well-defined
plugin API. Static plugins are built with console and included in the console
image. There are currently over a half-dozen plugins.

In addition to static plugins, we'd like to introduce dynamic plugins. Dynamic
plugins are contributed by operators and loaded at runtime. Eventually we want
to migrate static plugins to dynamic plugins to decouple them from the console
and allow plugins to be updated more frequently.

## Motivation

Static plugins have worked well, but they have some limitations:

1. Static plugins can only be updated on the OpenShift release cadence. We want
   to enable plugin teams to move faster.
1. Static plugins are tied to the console version, not the component operator
   version. We want to be able to update plugins as operators change APIs or
   add features. The right version of the plugin should be used for the
   installed version of the operator.

We already have a half-dozen teams contributing to console. We need to be able
to scale as more teams contribute. This will be difficult if everyone is
contributing to a single monorepo.

### Goals

* Operators can deliver console plugins separate from the console image and
  update plugins when the operator updates.
* The dynamic plugin API is similar to the static plugin API to ease migration.
* Plugins can use shared console components such as list and details page components.
* Shared components from core will be part of a well-defined plugin API.
* Plugins can use [Patternfly 4](https://www.patternfly.org/v4/) components.
* Cluster admins control what plugins are enabled.
* Misbehaving plugins should not break console.
* Existing static plugins are not affected and will continue to work as expected.
* Plugins can declare the version of the console framework they support and
  will be disabled when their version requirements are not met.

### Non-Goals

* Initially we don't plan to make this a public API. The target use is for Red
  Hat operators. We might reevaluate later when dynamic plugins are more
  mature.
* We can't prevent breaking changes in Patternfly APIs the console exposes to plugins.
* Plugins won't be sandboxed. They will have full JavaScript access to the DOM and network.
* This proposal does not cover allowing plugins to contribute backend console endpoints.

## Proposal

### Module Federation

Console will use [Webpack module federation](https://webpack.js.org/concepts/module-federation/)
to load plugins at runtime. Module federation allows a JavaScript application
to dynamically load code from another application while sharing dependencies.
It also allows console to share its components with plugins built and bundled
separately.

Plugins will need to be built with Webpack 5 to use module federation.

We have a pull request with a [working prototype](https://github.com/openshift/console/pull/6101)
that uses module federation.

### Shared Dependencies

Console will expose React and Patternfly 4 as shared dependencies to plugins.
Plugins can use any Patternfly component. Only a single version of React and
Patternfly will be loaded.

Plugins will not be able to use legacy Patternfly 3 components.

### Delivering Plugins

Plugins are delivered through operators. The operator will create a deployment
on the platform with an HTTP server to host the plugin and expose it using a
Kubernetes `Service`. The HTTP server serves all assets for the plugin,
including JavaScript, CSS, and images. The `Service` must use HTTPS and a
[service serving certificate](https://docs.openshift.com/container-platform/4.4/security/certificates/service-serving-certificate.html).
The console backend will proxy the plugin assets from the `Service` using the
service CA bundle.

Operators declare that they have a console plugin available by creating a
cluster-scoped `ConsolePlugin` resource that includes the service name, port,
and path to the plugin manifest.

```yaml
apiVersion: console.openshift.io/v1alpha1
kind: ConsolePlugin
metadata:
  name: my-plugin
spec:
  displayName: My Plugin
  service:
    name: my-console-plugin
    namespace: my-operator-namespace
    port: 8443
    manifest: /manifest.json
```

Plugins are disabled by default. They need to be manually enabled by a cluster
administrator before console loads any plugin code. Console provides a UI
for enabling plugins, and the list of enabled plugins is set in the console
operator config.

```yaml
apiVersion: operator.openshift.io/v1
kind: Console
metadata:
  name: cluster
spec:
  managementState: Managed
  plugins:
  - my-plugin
```

This will trigger a new rollout of the console `Deployment` with the updated
plugins. Note that this does mean the user will need to refresh the browser
to see the new plugins.

An operator can also give console a hint that it has a plugin available through
the `console.openshift.io/plugins` annotations. The annotation tells the
console to show a checkbox to enable plugins during operator install. Only
users who can edit the console operator config will see the checkbox. The
annotation value is a serialized JSON array of strings, where each item is the
name of a `ConsolePlugin` resource the operator will create.

```yaml
apiVersion: operators.coreos.com/v1alpha1
kind: ClusterServiceVersion
metadata:
  annotations:
    console.openshift.io/plugins: '["my-plugin"]'
```

For operators from the `redhat-operators` catalog source, the install operator
page will check the plugin checkbox by default. For other operators, the
checkbox will not be checked by default. Any plugin can be enabled or disabled
later through the console cluster settings page.

Initially, the console will have an allowlist of plugin names that it will
load. Only plugins with those names will be loaded until we expand support.

### Loading Plugins

The console backend will proxy requests to the plugin `Service`. This way the
`Service` itself does not need to be exposed outside the cluster, and avoids
any same-origin or certificate errors in the browser loading JavaScript from a
separate domain.

When loaded in the browser, console will read the manifest for each plugin. The
manifest is a JSON document that contains metadata about the plugin and the
extensions. `extensions` mirrors the API used for static plugins, and the same
extension points will be available.

```json
{
  "$schema": "/path/to/schema/manifest.json",
  "id": "my-plugin",
  "version": "1.2.3",
  "pluginAPI": "4.7.x",
  "displayName": "My Plugin",
  "extensions": [
    {
      "type": "console.flag",
      "properties": {
        "handler": {
          "$codeRef": "example.testHandler"
        }
      }
    },
    {
      "type": "console.flag/model",
      "properties": {
        "flag": "EXAMPLE",
        "model": {
          "group": "example.com",
          "version": "v1",
          "kind": "ExampleModel"
        }
      }
    }
  ]
}
```

An extension may contain code references, encoded as object literal
`{ $codeRef: string }`. The value of `$codeRef` is `moduleName.exportName` or
`moduleName` (equivalent to `moduleName.default`). Webpack itself will resolve
the remote module when console calls `container.get` on the
[remote container](https://webpack.js.org/concepts/module-federation/#dynamic-remote-containers).

#### Plugin Manifest Properties

| Property | Description |
| ----- | -------- |
| `name: string` | Used as a unique identifier. Each plugin must have a unique name. |
| `version: string` | The version of the plugin. Version must be parsable by node-semver, eg. “1.2.3” |
| `pluginAPI: string` | Semver range of compatible OpenShift console versions. |
| `displayName?: string` | User friendly display name. |
| `description?: string` | The description of the plugin. |
| `extensions: Extension[]` | List of extensions contributed by the plugin. |
| `dependencies?: { [pluginName: string]: string }` | A plugin which depends on other plugins must explicitly define their dependencies. The key of the map is the plugin name and the value is a semver version range. The plugin is not loaded if a dependency is not met. |

Property types are expressed as TypeScript types.

##### Extension<P = JSON>

| Property | Description |
| ----- | -------- |
| `id: string` | Unique identifier of this extension. |
| `type: string` | The `type` identifies this extension of a particular extension type. |
| `properties: P` | The extension properties JSON. |
| `flags?: Partial<{ required: string[]; disallowed: string[]; }>` | Reference to the flags used to trigger the enablement of this extension. |

### Lazy Loading

Console uses
[code splitting](https://webpack.js.org/guides/code-splitting/) to only load
JavaScript code when needed. Likewise, dynamic plugin code will only load when
needed. For instance, if the plugin contributes a list page, the plugin code
for that list page only loads when the user visits the page. This improves
performance and guards against bugs in plugins the user is not actively using.

Console performs feature detection on initial load. This is used today to
enable or disable parts of the UI based on features in the cluster and is
typically driven by the resources found during API discovery. Dynamic plugins
will also be able to declare console feature flags and will only load if the
flag is enabled. For instance, the KubeVirt plugin will only load if the
`VirtualMachine` resource is present.

### Refreshing When a Plugin Updates

Dynamic plugin code won't update in the browser automatically when a plugin is
added, removed, or updated. The user must refresh the browser.

The console backend will expose an endpoint that lists the available plugins
and the current plugin version from each manifest. The frontend will poll this
endpoint periodically to detect changes. When a change is detected, console
shows a message in a toast notification indicating that there is an update
available and the user must refresh their browser to see the changes. (We won't
refresh the page automatically to avoid possibly losing data if the user is
entering something into a form or the YAML editor.)

### Error Handling

Console will guard against runtime errors in plugins. All plugin components
will be surrounded by [React error boundaries](https://reactjs.org/docs/error-boundaries.html).
This prevents an uncaught error from causing the application to white-screen
and break. If a plugin service is unavailable, console will not load the plugin
and show a message in the notification drawer to let users know.

Additionally, the console can contain a `?disable-plugins` query parameter with
a comma separated list of plugin names. When this parameter is present, the
corresponding dynamic plugins are not loaded. If `all` is passed, all dynamic
plugins will be disabled.

### Risks and Mitigations

**Risk**: Patternfly updates with breaking API changes can break plugins.

**Mitigation**: Plugins can declare a semver range for compatibility. The core
console team will coordinate breaking changes with the plugin teams so that a
new operator version will be available with fixes when the corresponding
OpenShift version GAs. We will not ship any breaking Patternfly changes in a
z-stream.

**Risk**: Plugins are not sandboxed and can make any API request on behalf of the user.

**Mitigation**: Plugins are not enabled by default. A cluster admin must opt-in.

**Risk**: Customers/ISVs will use this API before it's fully supported.

**Mitigation**: Initially, we'll have an allowlist of supported plugins to make
it clear that only these plugins should be installed.

## Design Details

### Test Plan

In some ways, dynamic plugins will simplify how we test since each plugin has
its own repo. Plugins like KubeVirt and OCS have special requirements that can
be managed in their own merge queues.

The core console repo will run e2e tests on a sample plugin that exercises the
dynamic plugin extension points. The tests will install the sample plugin and
verify that the contributions to different areas of the UI work as expected.
We'll make the sample plugin available as a reference for other teams
developing dynamic plugins.

Additionally, we'll add API test suites to make sure we don't break API compatibility.

Manual testing of specific plugins like OCS and KubeVirt will continue as
before, including regression testing for any plugin migrated from static to
dynamic.

### Graduation Criteria

Initially, this will be an internal API. We will reevaluate as we receive
implementation feedback from plugin teams and as the dynamic plugin model
matures.

Static plugins are already a supported feature. Any existing static plugin that
is migrated to a dynamic plugin will need to have the same support level.

### Upgrade / Downgrade Strategy

We'll need to make sure any static plugin we remove has a dynamic plugin ready.
OLM index images where operators are locked to a specific OpenShift version
will allow us to do this. We can make sure the right operator level is required
for the release where we remove the static plugin. The console operator will
default well-known plugins to enabled when transitioned from static to dynamic
on cluster upgrade if the operator is installed.

A plugin might be unavailable for a window during a cluster upgrade if the
console version and operator are at different versions.

### Version Skew Strategy

We cannot prevent dependencies like Patternfly from releasing API breaking
changes, so we need a way for plugins to specify version compatibility. This
can also be helpful for operators who depend on new plugin APIs that are only
available in certain OpenShift versions.

Plugins can declare a compatible OpenShift semver version range in the plugin
manifest using the `pluginAPI` property. The console will only load plugins for
compatible OpenShift versions.

An operator could contribute multiple plugins with different version ranges to
support different OpenShift versions. If the version ranges don't overlap, the
console will only load the correct plugin.

## Implementation History

* 2020-07-24 - Initial [dynamic plugin PR](https://github.com/openshift/console/pull/6101) opened.

## Drawbacks

* It will be harder to coordinate changes across core console and plugins since
  they no longer share a repo.
* We have to handle version skew between plugins and core console.

## Alternatives

RHACM has an architecture where each component is a microservice that
contributes its own UI behind a single ingress with a shared masthead and
navigation. We could adopt this architecture for OpenShift console, and it
would allow us to more easily integrate RHACM and OpenShift console.

Drawbacks to this approach for OpenShift console:

* It would require significant rework to both core console and plugin code
  since console has a different architecture today.
* It might not be possible to achieve feature parity with current static
  plugins since they extend existing console pages. For instance, the OCS
  plugins contributes dashboard tabs to the Home -> Overview page to add
  dashboard tabs, and the Knative plugin adds capabilities to the developer
  topology view.

We could consider a hybrid approach where RHACM and OpenShift console
integrate in this way, but we support module federation for the existing
console plugins.
