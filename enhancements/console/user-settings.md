---
title: user-settings
authors:
  - "@cvogt"
reviewers:
  - TBD
  - "@bparees"
  - "@spadgett"
approvers:
  - TBD
  - "@spadgett"
creation-date: 2020-09-03
last-updated: 2020-09-03
status: implementable
---

# User Settings for OpenShift Console

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [x] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

The OpenShift Console has many features targeting different personas; from the Administrator to the Developer. There are limited features in Console today that save settings in order to enhance the user experience upon returning to any particular page. These settings are stored in browser local storage and therefore are not accessible in another browser or machine. Many features that could benefit from customization do not provide customization today due to lack of persisted user settings.

By introducing persisted user settings, many new and existing features will be designed with user settings in mind. Providing a richer experience which the user can tailor to their wants.

## Motivation

After talking with a number of customers, they often use OpenShift Console over a number of machines. They spend time setting up the experience in one browser session and computer, to only have to recreate it on the others. This if often the case due to desktop computers vs lab computers vs home computers. This becomes increasingly important as more features tailor the user experience to the individual.

### Goals

- Provide a mechanism for Console features to store and retrieve user settings.
- Securely store user settings such that access is restricted to the individual user.
- Save and restore user settings across different machines and browsers.

### Non-Goals

- Determining which features should make use of this functionality over local and session storage.

## Proposal

### Access Control and Storage

A single `ConfigMap` per user will be used to store user settings. The Console will require a new backend service to manage the creation of and access to the `ConfigMap` on behalf of the user.

User settings `ConfigMaps` will be stored in the `openshift-config-managed` namespace.

The console service account will be used to create, fetch, and update the `ConfigMaps` thereby preventing unauthorized access by any arbitrary user.

Each user settings `ConfigMap` will have a generated name using `metadata.generateName: user-settings-`.

In order to locate the `ConfigMap` associated to the user, labels will be used:

- `console.openshift.io/user-settings: true`: identifies the `ConfigMap` for the purpose of user settings
- `console.openshift.io/user: <user UID>`: associates the `ConfigMap` with its user by their unique identifier

Requires a nhe new role binding for the console service account to grants access to `create`, `update`, and `patch` for `ConfigMap` resources in the chosen namespace.

### Backend API

The Console backend will handle all user settings requests through `/api/console/user-settings`. Since the user cannot directly create or update the `ConfigMap` resource, the backend will need to first fetch or create the users' `ConfigMap` in order to identify and update the correct resource.

Initially support methods: `GET`, `PUT`, `PATCH`
Depending on how settings are used by some features, we may also require support for watching the resource for changes.

The backend service will lazily create exactly one `ConfigMap` per user. The Console frontend will query the backend service to fetch the contents of the `ConfigMap` when needed.

### Settings Keys

While the Console frontend will be the only consumer and producer of user settings, many features of the Console will contribute to user settings. This includes future dynamic contributions through extensibility. Therefore it is important to follow conventions for key creation in order to avoid collisions.

Settings keys should be uniquely qualified using reverse domain notation:

- `console.homepage`
- `dev-console.topology.filters`

### User Stories

#### Story 1

As a user, I want my guided tours progress to be saved with my account, such that returning to the Console on another machine will show my progress through each tour.

### Risks and Mitigations

**Risk**: Constraining user settings to only be accessible by their intended user.

**Mitigation**: RBAC rule to only allow the console service account access to read and write the user settings and all user requests proxied through the Console backend.

**Risk**: User settings keys between features may collide.

**Mitigation**: Enforce qualified keys where possible and document best practice for creating unique keys.

## Design Details

## Open Questions

- [ ] Is `ConfigMap` the best option, or a new `ConsoleUserSettings` CRD?
- [ ] Which namespace to store user settings `ConfigMap` resources?
- [ ] How to handle cleaning up stale user preferences?

### Test Plan

Testing will be carried out with the usual Console e2e and unit test suites.

### Graduation Criteria

None

### Upgrade / Downgrade Strategy

Support for this feature requires no special handling during upgrade. Default user settings will be provided.

### Version Skew Strategy

No concerns with version skew. Console will be the only producer and consumer of the user settings.

## Implementation History

None

## Drawbacks

- A resource must be maintained per user.

## Alternatives

The alternative is to continue to use browser local storage. While it is possible to store all the users' settings in the browser, the settings cannot be shared across different browsers or machines.
