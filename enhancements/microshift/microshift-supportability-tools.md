---
title: microshift-supportability-tools
authors:
  - pacevedom
reviewers:
  - "@copejon, MicroShift contributor"
  - "@fzdarsky, MicroShift architect"
  - "@ggiguash, MicroShift contributor"
  - "@oglok, MicroShift contributor"
  - "@pmtk, MicroShift contributor"
  - "@TurboTurtle, SOS contributor"
  - "@jrthms, product experience liaison"
approvers:
  - "@dhellmann"
api-approvers:
  - None
creation-date: 2023-01-17
last-updated: 2023-02-15
tracking-link:
  - https://issues.redhat.com/browse/USHIFT-389
see-also:
  - "/enhancements/oc/must-gather.md"
---

# MicroShift supportability tools
## Summary
This enhancement proposes the addition of supportability tooling to MicroShift.

To debug issues in the cluster we need a single command that any user or
administrator is able to run to gather all the required information to diagnose
and solve problems.

## Motivation
When it comes to supportability, having a tool to extract all relevant
information from a cluster becomes paramount to solve issues.

Most readers will be familiar with `must-gather` for OpenShift clusters.
MicroShift does not have such a tool as of today, and it becomes increasingly
important as it approaches general availability.

The ultimate goal is to have a single command to produce a full report of the
cluster status.

### User Stories

* As a MicroShift administrator, I want to be able to get a report about
  current cluster status that I can attach to a Red Hat support case.
* As a CEE engineer, I want to receive a comprehensive report that I can
  analyse and investigate to diagnose potential issues.

### Goals
* Provide a single tool to automate the report creation for the device edge
  product, not just MicroShift.
* Produce MicroShift collection output in a format compatible with CEE tools
  such as omg/omc.
* Limit the size of support applications stored on disk and pulled over the
  network at runtime.

### Non-Goals
N/A

## Proposal
MicroShift is one of the components of Red Hat Device Edge solution. This has
several implications:
* Runs on top of the OS, like another application.
* It is optional.
* Does not own nor manage underlying OS.

Much of MicroShift's relevant data is not available/reachable without logging
into the system. System wide configuration is an input to MicroShift, not
something it handles and mutates.

These characteristics drive the solution towards [sos](https://github.com/sosreport/sos),
which is a tool for collecting system wide logs and debug information.
Extensions to support additional applications are made through specialized
plugins. This document proposes the addition of a new plugin to produce
MicroShift's relevant data within a system report.

### Must gather
[must-gather](https://github.com/openshift/must-gather) is a widespread tool
for OpenShift clusters. Running from the `oc` client as another command, it
creates a debugging pod on the master node to gather information about the
hosts. It also collects information about OCP resources from specific system
namespaces.

While this tool suits MicroShift's needs, there are several caveats:
* Needs another image. This goes against one of the design principles of
  MicroShift, which is to be as light as possible. Some deployments might not
  be capable of pulling images from a remote registry, meaning they need to be
  preloaded when installing MicroShift. Either option is not the best when it
  comes to MicroShift.
* Since `must-gather` was specifically made for OCP, it is tailored to
  run in such solution. There are many specific resources and system components
  that MicroShift does not have, triggering errors. Some of the relevant system
  components for MicroShift are not checked in `must-gather` because they have
  a different name, or don't exist.
* Adapting `must-gather` to run in MicroShift is not a trivial task, as it not
  only requires MicroShift awareness, but also violates some of the
  MicroShift's principles, like lightness. It would be debatable if the tool
  should have such intelligence/complexity.

It is desirable to have the MicroShift tool match the output format of
`must-gather`, as there are several tools easing debuggability:
* [omg](https://github.com/kxr/o-must-gather)
* [omc](https://github.com/gmeghnag/omc)

Plus, CEE is used to this format when it comes to reports.

### Sos
[sos](https://github.com/sosreport/sos) is a data collection tool targetting
any UNIX based system. It is composed of specialized plugins which gather all
the relevant data from the different components and applications in a system.

Executing `sos report` will generate a detailed report with all the enabled
plugins (listed by running `sos report -l`).

A [plugin](https://github.com/sosreport/sos/wiki/How-to-Write-a-Plugin) is a
python script included as part of `sos` and makes the tool extensible. Plugins
also have an enablement check to run by themselves upon a report command, or
may be invoked manually.

Among the plugins already available we can find one for [OpenShift](https://github.com/sosreport/sos/blob/main/sos/report/plugins/openshift.py).
This plugin relies on the usage of `oc` command line tool and it gathers data
from OpenShift API resources, collecting logs for pods and kubelet. While
MicroShift shares similarities with this plugin it is not directly usable, as
the APIs are not the same, resources are not the same, and the output format
is required to follow `must-gather` format.

Since none of the available plugins fulfills MicroShift's needs, we will need a
new one.

Requirements for the `sos` plugin follow:
* Not require usage of other tools than what is already present in the host.
  No additional images or componentes shall be pulled when performing a report.
* Have `must-gather` format to use already existing debug tools.
* Retrieve MicroShift specific resources only, not covered by other plugins.
  These include cluster resources, MicroShift configuration, MicroShift
  version and MicroShift logs. Some specifics, like ovnk will require an
  additional plugin, as openshift does. Other components, such as CRI-O, have
  their own plugins.
* Provide a single command pre-configured for full report gathering.
  In order to have a single and simple command to gather the report, provide a
  profile configuration file for `sos` to run all relevant plugins in one shot.
* Must not leak sensitive data: IPs, hostnames, passwords, certificates, etc.
  The /var/lib/microshift directory holds certificates and etcd data, therefore
  it must be skipped.

Existing debugging tools in use by CEE include [omg](https://github.com/kxr/o-must-gather)
and [omc](https://github.com/gmeghnag/omc). These provide a CLI like experience
in terms of getting information from a live cluster, ressembling `oc` commands,
but coming from a must-gather report. Since they were born out of must-gather,
they expect the same file structure layout. The must-gather file system
hierarchy is driven by `oc adm inspect` commands, gathering cluster resources
data that these tools read upon later.

To achieve the same results the most simple approach is to include the `oc`
binary with MicroShift as a dependency. The plugin can then call
`oc adm inspect` to produce the output required. There is no API between the
plugin and MicroShift, easing maintainability and allowing enough flexibility
for the future.

The plugin is capable of automatic detection of installed packages, running
services and running containers in the system. By performing these checks it
can determine if there should be data collection for MicroShift without
providing additional flags and/or configuration files.

### Workflow Description
> _Disclaimer:_ Workflows described here illustrate manual processes.

Assuming from now on MicroShift has already been installed in a R4E/RHEL based
system and there has been some kind of issue.

1. MicroShift administrator notices an issue/wants to get a system report.
1. The administrator will log in to the failing host.
1. The administrator will perform the debug report creation procedure:
```bash
sudo sos report
```

For CI jobs the same procedure applies. This shall be automated upon failure.

The following situations may appear when needing to run `sos report`:
1. Healthy MicroShift. A running service where apiserver is able to answer
   requests and all control plane components are running is the perfect
   situation for `sos report`. All the required info will be gathered.
1. Failing MicroShift. A failing MicroShift deployment will not be able to run
   the apiserver component, therefore all the `inspect` logic will be offline.
   However, non-failed pods should keep their containers running, and their
   info is still collectable from other plugins, such as crio. Crio plugin is
   enabled by default in the presence of MicroShift.
1. Failing base OS. If the base OS fails for whatever reason (including an
   upgrade), then the `sos` report should focus on diagnosing what went wrong
   at a wider level. In this situation MicroShift could be running or not,
   depending on the underlying failure. In any case, a full report should help
   diagnose issues down to the lowest level, as the plugins enable themselves
   automatically based on local services and packages. If the issue ends up
   impacting MicroShift, it falls into any of the other categories in this
   section.
1. Certificates failure. In the event of having a certificate failure the
   apiserver is also unreachable, even if it is running. This is the same
   situation as the failing MicroShift above.

#### Variation [optional]
N/A

### API Extensions
N/A

### Implementation Details/Notes/Constraints [optional]
N/A

### Risks and Mitigations
There are four main risks:
1. Non backwards compatible changes.
1. `sos` release cadence.
1. CEE having a different tool.
1. Exposing sensitive data.

#### Non backwards compatible changes
MicroShift does not yet support upgrades but the plugin should be able to
handle them from the beginning. In the event of having incompatible changes
between MicroShift releases the plugin should be ready to support it.

To try to minimize the effects of incompatible changes the plugin should
retrieve resources in the most generic way that guarantees future
compatibility. This makes it resilient to breaking changes being introduced,
such as namespace name changes, or deprecating resources. If any failure should
happen the plugin must try to retrieve as much information as it can and log
the errors for later analysis. Plugin execution must not stop on errors.

MicroShift includes `sos` as a dependency, and given the shorter release cycle
(see next section) in `sos`, both components should always be in sync.

Issues when upgrading the base OS should be handled at OS level, as MicroShift
is an application running on top of it.

#### Sos release cadence
`sos` release cadence has been shortened to 4 weeks starting on Feb 1st 2023.
This change has been introduced to allow faster support of new components and
features. Since MicroShift follows the same release calendar as OpenShift,
`sos` is faster.

New versions of `sos` will simply slot in to the next regular release of both
RHEL 8 and 9. If there is an urgent need a hotfix can be made between releases.

#### CEE having a different tool
CEE might have concerns over using a different tool than `must-gather`. `sos`
reports are able to extract a full system report, as opposed to `must-gather`
which is only suitable for OpenShift clusters.

Moreover, there is a growing number of plugins to support anything that runs
on a RHEL system, making it a suitable go-to gathering tool when it comes to
diagnosing system-wide issues.

The microshift plugin for `sos` shall keep the same output format when it comes
to cluster resources than `must-gather` in order to minimize differences and
allow the same tooling that is currently used for OpenShift.

#### Sensitive data
Tool shall not dump contents from certificates and/or any other sensitive data.
There is a `--clean` option which obfuscates hostnames, IP addresses, MAC
addresses, and any text matching user defined input.

### Drawbacks
Discussed in risks and mitigations.

## Design Details
Refer to `sos` [documentation](https://github.com/sosreport/sos/wiki/How-to-Write-a-Plugin)
for more information on how to create plugins.

Plugin relies on the execution of `oc adm inspect` to produce the report. To
do so it needs to use the appropriate parameters when calling. Here we can
distinguish 2 resource categories:
* Cluster scoped resources.
* Namespaced resources.

The former will have a 2-pass filter: first iterate over a list of common
resources to check for existence, then execute `oc adm inspect` over the ones
that returned results.
The latter will execute `oc adm inspect ns/<namespace name>` over system
namespaces: those with `kube-` or `openshift-` prefixes.

Since the plugin is automatically activated by installing/running `microshift`,
the journal logs and other basic info, like version and configuration, are
always retrieved. To execute `oc adm inspect` we need to have the apiserver
running, which is equivalent to check the service status and act accordingly.

On a full run, this is the output format:
```bash
sosreport-microshift-2-2023-02-16-eyxlytj/
├── sos_commands
│   ├── microshift
│   │   ├── cluster-scoped-resources # oc adm inspect format follows from here.
...
│   │   ├── inspect_cluster_resources.log # oc adm inspect cluster wide resources output logs.
│   │   ├── inspect_namespaces.log # oc adm inspect namespaces output logs.
│   │   ├── journalctl_--no-pager_--unit_microshift # sos plugin service logs.
│   │   ├── namespaces # oc adm inspect format follows from here.
...
│   │   ├── systemctl_status_microshift # sos plugin service status.
│   │   └── timestamp # time at which plugin was executed.
...
```
More information on the `oc adm inspect` [format](https://github.com/openshift/enhancements/blob/master/enhancements/oc/inspect.md#output-format).

### Test Plan
MicroShift-only tests should be added to verify the output structure: dirs and
files. The contents may be checked only at the hierarchy and size level.

An e2e test should test the command exits successfully and certain contents are
always present.

Once the plugin is introduced, it will also be used as part of the debug logs
for all failed jobs in the MicroShift CI.

The happy path tests should run on the `sos` CI, going alongside the plugin.

There should also be an additional test to crash MicroShift on purpose and check
the `sos report` contents are enough in the absence of some working component.

### Graduation Criteria

#### Dev Preview -> Tech Preview

* Have the plugin as part of `sos` released in RHEL9 repos.
* QE and CEE feedback.
* Initial test coverage.

#### Tech Preview -> GA

* User facing documentation.
* Introduce for all other CI jobs.
* Complete test coverage.

#### Removing a deprecated feature
N/A

### Upgrade / Downgrade Strategy
This is handled at the OS level, as `sos` is a system package.

### Version Skew Strategy
`sos` package is released on RHEL repos. Including it as a dependency for
MicroShift should ease any upgrade/downgrade scenario, as it will install it
before running MicroShift.

Still, the `sos` plugin should always strive to retrieve the information in the
most generic way possible. This way we minimize possible problems when
releasing or installing any of the two components.

`sos` releases on both RHEL8 and RHEL9 repos with the same version. When it
releases, it does with the same version in the current RHEL 8/9 versions. If
8.7 is the latest, `sos` x.y+1 will get released in 8.7 but not necessarily in
8.6, which would require a specific request for a backport.

### Operational Aspects of API Extensions
N/A

#### Failure Modes
N/A

#### Support Procedures
N/A

## Implementation History
N/A

## Alternatives
### oc adm must-gather
[must-gather](https://github.com/openshift/must-gather) is the tool of choice
for OpenShift. While this tool covers all that has been discussed in this
enhancement, it assumes the target is cluster is a full blown OpenShift
deployment.

On top of the errors, the report is empty for many components because it
expects certain names and namespaces to exist, which don't in MicroShift.

As discussed in the proposal, in order to use `must-gather`, we would need
to add MicroShift awareness to the tool.

### Import inspect packages into MicroShift
The `inspect` command is part of [oc](github.com/openshift/oc) packages, so it
can be imported into other projects and handle it as any other dependency.

Importing this package into MicroShift implies potential additional work in
the form of one more dependency that requires tracking for rebase scripts.
There is also the issue of version sprawl between dependencies, as some
indirect packages may introduce compile or runtime errors. Importing `inspect`
yields the need for a patch already because of an incompatibility with a
kubernetes package.

### Mimick inspect command
Instead of importing `oc adm inspect` command into MicroShift, the plugin can
mimick its behavior so that it produces the same output. This may be seen as
another implementation for inspect, only in another language.

Producing the same file hierarchy in an efficient way involves not only the
grouping that `oc adm inspect` does for resources (core resources, for example)
but also doign as few calls to `get` as possible. The best possible scenario
uses a single `get` command to get all resources in one shot, then requires
heavy parsing and manipulation to produce all files according to the
`omc`/`omg` layout.

Retrieving logs would still be inefficient, as it is needed to issue two `get`
commands per container per pod.

### Use a different output format
Not including `inspect` in `microshift` and not following `must-gather` output
would allow the plugin to produce information in any possible format. When CEE
get the report, though, they would like to use `omc` or `omg`, which rely on
that specific format, as it can be seen in the goals.

Using any of those tools would require a new tool to translate the plugin
output to `must-gather` format. Another element into the toolbox, it has all
the implications that any piece of software has: owners, development,
maintainability, releases, etc.

### Use a separate inspect binary
Adding `oc` binary increases footprint by about 120M once installed. Another
possibility would be to create a standalone binary with packages for
`oc adm inspect` command, and call this instead. The resulting binary would
account for a bit over 40M in size, which is only a fraction of `oc`, but this
comes with its own set of issues.
The binary would be able to do only one task, which is inspect, and it needs
dependency management in source code, plus release strategy, etc.

`oc` has already dealt with all of the subtleties associated with a new
component in the system and is ready to use after a simple install.
For the time being MicroShift is ok to add this small footprint for the
benefits it provides.

## Infrastructure Needed [optional]
N/A
