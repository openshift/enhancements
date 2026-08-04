# OpenShift Development Phases

This document outlines the phases of OpenShift development and the guidelines
around merging code and where we expect teams to focus. OpenShift is a very
large and complex product with a lot of engineers and a lot of moving parts. We
want to encourage the org to move as fast as it can, but no faster. We rely on
our CI systems and payloads as key indicators of when we’re moving too fast and
finding gaps in pre-merge testing. We have a historical well documented pattern
of a last minute rush and teams feeling pressure that their work must land in a
given release, and a long standing goal to get better at pacing ourselves and
focusing on what’s most important.   

## Goals:
1. Encourage and increase the use of Feature Gates.
2. Reduce the rush of features merging very late, leading to CI instability.
3. Increase focus on release blocking features, and normalize deferring non-blocking features to the next release. 
4. Reduce pressure to land non-blocking features by allowing backports to early z-stream releases.


## Guidelines

NOTE: See [How to file an SBAR](https://docs.google.com/document/d/1-Lq4p7KhHRUFhkhpZ1ntDOcvDZgj9YVIBOmLSRlNkq0/edit?usp=sharing) and the template linked within if you need to file an SBAR per the guidelines below.

| Phase | Development | Stabilization | Post-Branching / Feature Freeze | Post GA |
| :--- | :--- | :--- | :--- | :--- |
| **Focus** | Ranked/prioritized feature development.<br>Getting features to done and limiting WIP.<br>Relatively rapid response to regressions. | Bug fixes, stabilizing CI.<br>Landing release blocking features, if possible.<br>Exercising caution on anything merging at this time.<br>Ack critical fixes or staff eng approved label requirements may be turned on if CI is struggling. | Bug fixes.<br>Ack critical fixes may be turned on if the pending release blocking bug count is too high, to allow for fixes to land smoothly for backporting.<br>Proceed with feature work for next release, avoid getting anchored to prior whenever possible. | Bug fixes.<br>Minimal backporting of feature gates and promotion for features that nearly made it in for GA.<br>Proceed with feature work for next release, avoid getting anchored to prior whenever possible. |
| **Gated Release Blocking Features** | Can merge and promote per normal processes. | Can merge code and [promote via normal processes](https://github.com/openshift/enhancements/blob/master/dev-guide/featuresets.md#id-like-to-declare-a-feature-accessible-by-default--what-is-the-process).<br>Promotion requires additional approval from an OCP architect. No SBAR required.<br>Logically impossible gaps in test coverage can be approved by API reviewers/architects.<br>Any other gaps in testing require an SBAR. | SBAR required to promote. | Can pursue a backport to land in an early z-stream release:<br>SBAR required, submitted once release is GA and feature is promoted to default in main.<br>Wait for SBAR approval before beginning backporting work. |
| **Gated Non-Blocking Features** | Can merge and promote per normal processes. | Can merge code and promote via normal processes.<br>Promotion requires additional approval from an OCP architect. No SBAR required.<br>Logically impossible gaps in test coverage can be approved by API reviewers/architects.<br>Any other gaps in testing require an SBAR. | Defer to next release, and possibly an early z-stream. | Can pursue a backport to land in an early z-stream release:<br>SBAR required, submitted once release is GA and feature is promoted to default in main.<br>Wait for SBAR approval before beginning backporting work. |
| **Non-Gated Release Blocking Features** | Can merge and promote per normal processes. (but feature gates are always preferred) | SBAR required. | SBAR required. | N/A<br>Ideally we would not want to backport features to z-stream releases that did not have a feature gate. (we cannot validate them in techpreview there before shipping) |
| **Non-Gated Non-Blocking Features** | Can merge and promote per normal processes. (but feature gates are always preferred) | Defer to the next release.<br>May be reverted/held by architects.<br>SBAR is required if you want to attempt merging a non-blocking feature, but bias will be towards deferring. | N/A | N/A |

## What does it mean if I have to defer?

Once the release is GA, teams can pursue a backport to land in an early z-stream for a deferred feature by submitting an SBAR. We encourage this to only be pursued when absolutely necessary to avoid getting anchored to the prior release.


