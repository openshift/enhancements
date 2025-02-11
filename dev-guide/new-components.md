---
title: Policies for Adding New Components
authors:
  - "@dhellmann"
reviewers:
  - ""
approvers:
  - "@derekwaynecarr"
creation-date: 2024-03-04
last-updated: 2024-03-04
status: informational
---

# Policies for Adding New Components

## General Guidelines

We are working to reduce the operational cost and overall footprint of
running an OpenShift cluster. Adding new components generally moves us
in the opposite direction of this goal. Consider whether a new
component really needs to be in the OpenShift payload, or whether it
can be delivered via OLM as an operator.

There are a few downsides to adding new components directly to
OpenShift:

* The new components are present in every cluster, even if that
  cluster does not need the features provided.
* Their release schedule is tightly coupled to the OpenShift release.
* Each new image we add increases the storage requirements and
  bandwidth needed to mirror images into disconnected settings.

## Projects with Existing Communities

Releasing community-driven projects with existing code bases, whether
by adding them to the OpenShift payload or delivering them via OLM, we
need to consider our obligations to provide timely fixes for CVEs. We
have two basic requirements to ensure we can meet those commitments:

* At least 2 Red Hat employees must have maintainer privileges within
  the project and be able to approve changes.
* Red Hat must have access to embargoed CVE notifications and fixes so
  we can prepare releases to coincide with upstream fixes being
  released.
