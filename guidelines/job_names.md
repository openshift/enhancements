# Job naming guidelines

Job names should indicate to the reader the purpose of the specific job,
as well as noting where the job deviates from OpenShift defaults for
that release. The guidelines contained in this document are based on the
way the majority of jobs are named today.

The general format of a job name should be as follows:

```
[JOB TYPE]-ci-[GITHUB ORG]-[GITHUB REPO]-[GITHUB BRANCH]-[STREAM]-[RELEASES]-[TEST SUITE]-[PLATFORM]-[CNI]-[OTHER DEVIATIONS]
```

When using multi-stage generated jobs, the first part of the name is
generated for you, and the second part that begins with "releases" is
determined by the job creator.

## Job name fields

**Job type**: Generally should be either "periodic" or "pull". Jobs that
begin with prefixes like "release" or  "promote" date back to
template-style jobs, which should generally not be used.

**GitHub org/repo**: The GitHub org and repo the job is configured for.
Release periodics typically use openshift/release, but teams may put
them under their own control subject to the guidelines below.

**Stream**: Which release stream is being tested, such as nightly or ci.

**Releases**: The X.Y release being used.  For minor upgrades, this
should be in the format "X.Y-upgrade-from-stable-4.(Y-1)". Upgrades that
test multiple versions are listed in sequence, i.e.
"4.10-to-4.11-to-4.12-to-4.13-ci". Micro upgrades do not need any
special designation other than the X.Y. All upgrade jobs should specify
"upgrade" under "other deviations."

**Test Suite**: If it is running a test suite from openshift/origin
(openshift-tests binary), this value should be “e2e.”
`openshift/conformance/parallel` is the assumed suite for "e2e" jobs,
unless otherwise specified in "other deviations" (such as "serial"). If
it is running something else such as tests from your own repo, you can
give it a descriptive name of your choice, but do not use the e2e
value. The OpenShift console tests use the value “console" for example.

**Platform**: This should be the cloud platform type. When the kind of
infrastructure (IPI, UPI, Assisted) is specified in the job name, this
must be conjoined with the platform (i.e., aws-upi).  Typically IPI
is assumed and is omitted.

**CNI**: The CNI provider, such as “ovn” or “sdn”

**Other deviations**:  Where the job deviates from OpenShift defaults
for that release, these should be noted in the job name.  Example
deviations include: “rt” (realtime kernel), “proxy”, "ipv6", and
non-amd64 architectures like "arm64".  If an e2e job that is running a
suite other than parallel, this should also be included here. For
example, “serial”.  Upgrade jobs should specify "upgrade" at the end of
the job name, whether it be a micro or minor upgrade.

### Example job names

- periodic-ci-openshift-release-master-nightly-4.13-console-aws
- periodic-ci-openshift-release-master-nightly-4.13-e2e-azure-ovn-etcd-scaling
- periodic-ci-openshift-release-master-nightly-4.13-e2e-metal-ipi-serial-ovn-dualstack
- periodic-ci-openshift-release-master-ci-4.13-e2e-gcp-sdn-techpreview-serial
- periodic-ci-openshift-release-master-ci-4.13-upgrade-from-stable-4.12-e2e-azure-sdn-upgrade

## When defaults change

When a default changes, for example in the upcoming switch from
runc->crun, jobs specifically designed to test "crun" should be removed.
New jobs may be created to test the "runc" configuration if it is
supported, and the old default should appear in these new jobs' names.

## Periodics outside of openshift/release

Typically, release payload informing and blocking jobs are configured in
the `openshift/release` repo under the
[openshift/release](https://github.com/openshift/release/tree/master/ci-operator/config/openshift/release)
configuration directory.  However, because teams may want more control over their
periodics, they may want to place them in their own repository's
configuration. Periodics should be placed in their
own configuration YAML file away from presubmit configuration, and the
release information should be set to the appropriate release version and
stream.  You may have multiple configurations per release branch by
using the [variants feature of ci-operator](https://docs.ci.openshift.org/docs/how-tos/contributing-openshift-release/#variants).

An example of this configuration is available
[here](https://github.com/openshift/release/tree/c1cf20f480b19e010e6581774452d579a60a92ed/ci-operator/config/openshift/cluster-control-plane-machine-set-operator),
where you can see the periodics are stored in the `__periodics.yaml`
files.

For these jobs to show up in [TestGrid](https://testgrid.k8s.io/) and
[Sippy](https://sippy.dptools.openshift.org/), the ci-tools
[configuration needs to be updated manually](https://github.com/openshift/ci-tools/pull/3261).

