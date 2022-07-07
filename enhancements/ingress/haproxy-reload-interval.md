---
title: haproxy-reload-interval
authors:
  - "@Ethany-RH"
reviewers:
  - "?"
approvers:
  - "?"
api-approvers: # necessary?
  - "?"
creation-date: 2022-07-01
last-updated: 2022-07-01
tracking-link:
  - "https://issues.redhat.com/browse/NE-586"
see-also:
replaces:
superseded-by:
---

# Reload Interval in HAProxy

## Release Signoff Checklist

- [ ] Enhancement is `implementable`.
- [ ] Design details are appropriately documented from clear requirements.
- [ ] Test plan is defined.
- [ ] graduation criteria for dev preview, tech preview, GA
- [ ] User-Facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/).

## Summary

Add an API field to configure OpenShift router's `RELOAD_INTERVAL` environment variable so that administrators can define the minimum frequency the router is allowed to reload to accept new changes.

OpenShift router currently hard-codes this reload interval to 5s. It should be possible for administrators to tune this value as necessary. Based on the processes run in the cluster and the frequency that it sees new changes, decreasing the minimum frequency that the router is allowed to reload when its configuration is updated can improve its efficiency.
This proposal extends the existing IngressController API to add a tuning option for max connections.

## Motivation

When there is an update to a route or endpoint in the cluster, the configuration for HAProxy changes, requiring that it reload for those changes to take effect. When HAProxy reloads, it must keep the old process running until all its connections die, so frequent reloading increases the rate of accumulation of HAProxy processes, particularly if it has to handle many long-lived connections.
