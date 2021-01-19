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
last-updated: 2020-10-02
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

The OpenShift Console has many features targeting different personas;
from the Administrator to the Developer. There are limited features in
Console today that save settings in order to enhance the user
experience upon returning to any particular page. These settings are
stored in browser local storage and therefore are not accessible in
another browser or device. Many features that could benefit from
customization do not provide customization today due to lack of
persisted user settings.

By introducing persisted user settings, many new and existing features will be designed with user settings in mind. Providing a richer experience which the user can tailor to their wants.

## Motivation

After talking with a number of customers, they often use OpenShift Console over a number of devices. They spend time setting up the experience in one browser session and computer, to only have to recreate it on the others. This if often the case due to desktop computers vs lab computers vs home computers. This becomes increasingly important as more features tailor the user experience to the individual.

### Goals

- Provide a mechanism for Console features to store and retrieve user settings.
- Securely store user settings such that access is restricted to the individual user.
- Save and restore user settings across different devices and browsers.

### Non-Goals

- Determining which features should make use of this functionality over local and session storage.

## Proposal

### Access Control and Storage

A single `ConfigMap` per user will be used to store user settings. The Console will require a new backend endpoint to manage the creation of the `ConfigMap` on behalf of the user.

User settings `ConfigMaps` will be stored in a new dedicated namespace: `openshift-console-user-settings`

An additional `Role` and `RoleBinding` per user will be created by the Console backend to restrict access to the `ConfigMap`. The `Role` will grant a user access to `get`, `patch`, `update`, and `watch` only their user settings `ConfigMap`.

All user settings resources will be named `user-settings-<user UID>` to prevent overlap with resources of another user.

The Console service account will require the RBAC necessary to create the user settings resources in the `openshift-console-user-settings` namespace.

#### Backend API

A new API in the Console backend will provide a facility for the frontend to initialize the user settings for the logged in user.

A `POST` request to this endpoint will create the 3 resources needed for the user's settings. After which, the frontend can use the standard kubernetes APIs to read and update the user settings `ConfigMap`.

### Settings Keys

While the Console frontend will be the only consumer and producer of user settings, many features of the Console will contribute to user settings. This includes future dynamic contributions through extensibility. Therefore it is important to follow conventions for key creation in order to avoid collisions.

Settings keys should be uniquely qualified using reverse domain notation:

- `console.homepage`
- `dev-console.topology.filters`

### User Stories

#### Story 1

As a user, I want my quick start progress to be saved with my account, such that returning to the Console on another device will show my progress through each quick start.

### Risks and Mitigations

**Risk**: Constraining user settings to only be accessible by their intended user.

**Mitigation**: RBAC rule to only allow the user access to their user settings `ConfigMap`. As well as a new endpoint that initializes the user settings resources on behalf of the user. No user will have access to create these resources themselves.

**Risk**: User settings keys between features may collide.

**Mitigation**: Enforce qualified keys where possible and document best practice for creating unique keys.

## Open Questions

- [ ] Is `ConfigMap` the best option, or a new `ConsoleUserSettings` CRD?
- [ ] Which namespace to store user settings `ConfigMap` resources?
- [ ] How to handle cleaning up stale user preferences?

### Test Plan

Testing will be carried out with the usual Console e2e and unit test suites.

### Graduation Criteria

None

### Upgrade / Downgrade Strategy

Some user settings today are stored in browser local storage. The Console frontend will implement support to merge these settings from local storage and override the defaults when no persisted user settings have been stored for the user.

Each key is defined by the code producing the user setting and is not
intended to be maintained as an API. When upgrading to a new version
of Console where the previous already had user settings, the onus will
be on the code consuming each individual user setting to manage their
own migration if the old value is no longer valid. This is the same
procedure used in previous Console releases when dealing with user
settings stored in local storage.

It is suggested that the value of each user setting remain consistent across versions of the Console and if the data type or meaning of the value were to change that a new key be used instead of overriding the old value.

Deletion of user settings should have no impact on the application other than the user setting reverting to its default value.

### Version Skew Strategy

No concerns with version skew. Console will be the only producer and consumer of the user settings.

## Implementation History

None

## Drawbacks

- Three resources per user are created.

## Alternatives

The alternative is to continue to use browser local storage. While it is possible to store all the users' settings in the browser, the settings cannot be shared across different browsers or devices.
