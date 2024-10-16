---
title: extended-platform-tests
authors:

- "@jupierce"
- "@stbenjam"
  reviewers:
- "@deads2k"
  creation-date: 2024-09-05
  last-updated: 2024-09-05
  status: implementable

---

<!-- TOC -->
* [OpenShift Tests Extensions](#openshift-tests-extensions)
  * [Release Signoff Checklist](#release-signoff-checklist)
  * [Summary](#summary)
  * [Motivation](#motivation)
    * [Goals](#goals)
    * [Non-Goals](#non-goals)
  * [Proposal](#proposal)
    * [Concepts](#concepts)
      * [Component](#component)
      * [Test ID](#test-id)
      * [Test Environment](#test-environment)
      * [Test Context](#test-context)
    * [Test Extension Binaries](#test-extension-binaries)
      * [Binary Discovery](#binary-discovery)
        * [OpenShift Payload Extension Binaries](#openshift-payload-extension-binaries)
        * [Non-Payload Extension Binaries](#non-payload-extension-binaries)
      * [Binary Format](#binary-format)
      * [Binary Extraction](#binary-extraction)
      * [Extension Interface](#extension-interface)
        * [Info - Extension Metadata](#info---extension-metadata)
        * [List Tests - Extension Test Listing](#list-tests---extension-test-listing)
        * [List Monitors - List which monitoring tests are available](#list-monitors---list-which-monitoring-tests-are-available)
        * [Run-Test - Running Extension Tests](#run-test---running-extension-tests)
        * [Run-Suite - Running Tests in Local Suites](#run-suite---running-tests-in-local-suites)
        * [Run-Monitor - Monitoring Cluster during Test Run](#run-monitor---monitoring-cluster-during-test-run)
        * [Config - Component Configuration Testing](#config---component-configuration-testing)
      * [Update - Metadata Validation](#update---metadata-validation)
      * [Extension Implementation](#extension-implementation)
      * [Test Result Aggregation](#test-result-aggregation)
    * [Risks and Mitigations](#risks-and-mitigations)
      * [Binary Incompatibility](#binary-incompatibility)
        * [CPU Architecture](#cpu-architecture)
      * [Runtime Size / Speed](#runtime-size--speed)
      * [Image Size](#image-size)
      * [Poor Extension Implementation](#poor-extension-implementation)
    * [Version Skew Strategy](#version-skew-strategy)
  * [Alternatives](#alternatives)
<!-- TOC -->

# OpenShift Tests Extensions

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Today all conformance tests for OpenShift are in the monolithic [openshift-tests]
(https://www.github.com/openshift/origin) binary. These tests run on all the possible
flavors of OpenShift (more than 50) and contribute signal to tools like the
Release Controller and Component Readiness. However, the high barrier to entry in
openshift-tests means that teams are either not writing tests, or are using their
own homegrown solutions that ran in only very narrow configurations.

This enhancement proposes a framework to allow allows external repositories to
contribute tests to openshift-tests' suites with extension binaries, keeping tests
colocated with the features they are testing. It defines a standardized interface
for test discovery, execution, and result aggregation, allowing decentralized
contributions while maintaining centralized orchestration.

## Motivation

Increase the number of tests in the OpenShift Tests Suite by reducing the
effort, complexity, and risk of introducing new tests.

### Goals

1. Reduce the effort, complexity, and risk of introducing new tests to OpenShift.
2. Allow tests to be introduced in the same pull request as the PR which fixes a bug or introduces a feature.
3. Allow component owners to introduce new test suites / add tests to existing suites.
4. Allow non-OpenShift components, including layered products, to participate in extending OpenShift tests and Component
   Readiness.
5. Maintain and increase testing coverage.
6. Maintain and increase code quality.
7. Maintain and improve the signal TRT provides through Component Readiness.
8. Provide a standard model for testing component configurations.
9. Allow distributed contribution of tests while introducing centralized mechanisms to require important tests from
   component owners and improve coverage over time.
10. Improve testing efficiency by reducing binary initialization time and parallelizing previously serial, but
    non-conflicting tests.
11. Simplify diagnostic data collection on test failure and allow it to be contemporaneous with that failure.
12. Allow centralized and methodical pre-submit testing of the test extension implementations. For example, preventing a
    test rename without preserving information about the original name.

### Non-Goals

1. Fully decompose `openshift-tests` or decentralize test orchestration & reporting.

## Proposal

### Concepts

#### Component

A component is a part of a software product. Component information includes the product name,
component name, and the type of the component. A single invocation of an external binary
can only associate with a single component (e.g. it can only list tests for / execute tests
for a single component).

#### Test ID

A unique combination of Component (Product, Type, Component Name) and original test name. 
A Test ID should not be repeated in a test listing across all participating extensions. 
The Test ID representing a unit of testing logic should not change over time (even if the 
human-readable test name changes, the Test ID should remain consistent by using the 
original test name).

#### Test Environment

A Test ID references a unit of testing logic. That logic may pass in some environments,
fail in others (e.g. a test that passes in gcp but fails in aws), and be
intentionally skipped in still others. A Test Environment
describes relevant facets of the environment in which a test would run or was run, including
configuration information.

Component Readiness may analyze test results using all, some, or none of this Environment
as dimensions along with Test ID to determine regressions, depending on the desired
insight (e.g. if the goal is to detect whether a test has regressed, on average, across
all environments, the environment will be ignored for the analysis).

#### Test Context

This is information known only by `openshift-tests` as a part of orchestrating test
execution. It includes information like the random seed being used for test execution
planning and the number of times a test has been run against a cluster.

Rarely, Component Readiness may analyze results using these dimensions to, for example,
determine that different runs of the same test on the same cluster behave differently
in a statistically significant way.

### Test Extension Binaries

Test extension binaries will be defined as a first class method of introducing new tests and suites
into OpenShift origin's test framework. Historically, all tests had to be contributed to
github.com/openshift/origin, but extension binaries can be developed in other repositories
and built independently of origin.

Extension binaries must implement an "extension interface" consisting of CLI verbs
and arguments that allow tests to be discovered and executed. The extension interface
is non-trivial to implement, so to simplify the creation of extensions, a Go module &
repository will be maintained by TRT: https://github.com/openshift-eng/openshift-tests-extension .

Vendoring this module and following its integration guide will provide component test authors
the majority of the logic necessary to fulfill the test extension interface. It will
also provide a means of centrally defining new testing requirements. As component owners re-vendor
the module, they would be required to implement those new testing requirements
for the module to compile and run a test extension binary successfully.

During a run of the origin test framework (`openshift-tests`), the framework will:

1. Discover available extension binaries.
2. Discover metadata and component configuration options for those extensions (running `info` verb and parsing standard
   out).
3. Discover tests available from those extensions (running `list` verb and parsing standard out).
4. Plan an appropriate and efficient testing strategy based on the framework invocation.
5. Execute the tests using subsequent invocations of the discovered binaries (`run-test` invocations).
6. Collect the result and output of the test invocations and integrate it into overall test suite results.

#### Binary Discovery

The existence of test extension binaries can be registered one of two ways by test authors.

##### OpenShift Payload Extension Binaries

For OpenShift payload components contributors can advertise the existence of an extension binary
by adding information (the imagestream tag for the OCP payload component and the path to the binary
within their image) to a simple registry datastructure in github.com/openshift/origin.

##### Non-Payload Extension Binaries

For non-payload components, contributors must advertise their extension binary for `openshift-tests`
to discover. This is accomplished by creating an ImageStream / ImageStreamTag with a special label:

`testextension.redhat.io/component=<product-name>-<component-name>`

An annotation on the ImageStreamTag will then identify the binary to extract and (optional)
arguments to pass ahead of the extension interface verbs.
`testextension.redhat.io/binary=<binary-path>.gz [--argument=value]`

Well-behaved operators should gate the creation of this imagestream/label/annotation on the existence of the
`TestExtensionAdmission` custom resource definition (the code should not attempt to
inspect `TestExtensionAdmissions` instances -- just check for the CRD itself so that operators
don't liter clusters with ImageStreams only applicable for testing in a production environment).

Optional Operator authors must ensure that the image carrying the extension binary
is identified in their ClusterServiceVersion (CSV) so that tools like `oc-mirror`
will copy image(s) bearing extension binaries to disconnected clusters.

Instances of `TestExtensionAdmission` gate which extension binaries `openshift-tests`
will use for test discovery. This is because extension binaries will be invoked
with a `system:admin` kubeconfig when tests are run. Administrators
must opt-in to this risk by instantiating one or more `TestExtensionAdmission`
objects.

`TestExtensionAdmission` contain a list of namespaces/imagestreams
from which `openshift-tests` is permitted to extract extension binaries. This
can include wildcard patterns for namespace, imagestream, or both.

```yaml
kind: TestExtensionAdmission
metadata:
  name: example
spec:
  permit:
  # All imagestreams in the openshift namespace with the testextension label
  - "openshift/*"
  # All imagestreams on the cluster with the testextension label
  - "*/*" 
```

`openshift-tests` will SKIP a single synthetic test whenever an extension binary is detected in
any imagestreamtag but which is not permitted for execution by a `TestExtensionAdmission`
instance. This will allow users to detect tests that are available but not yet explicitly
permitted to run.

#### Binary Format

Extension binaries are extracted from container images into a running Pod executing
`openshift-tests`. Contributors do not necessarily know which version of RHEL the
`openshift-tests` binary will be running on, so, for maximum portability, test
extension binaries should be statically compiled.

Statically linked binaries are prohibited in FIPS and will cause failures if
detected by product pipeline scans. To avoid this, extension binaries should be
gzipped before being committed to container images.

For compliance reasons, when a binary is compiled by a golang builder image
supplied by ART, a wrapper around the `go` compiler will force FIPS compliant
compilation (e.g. dynamic linking). For this reason, extension binaries
should include `GO_COMPLIANCE_POLICY="exempt_all"` in the environment when
compiling an extension binary. This will inhibit the wrapper from
modifying `CGO_ENABLED` or other compiler arguments required for static compilation.

#### Binary Extraction

After discovering a test extension binary, the origin test framework will extract the binary
from the container image which carries it and store it in /tmp storage of the pod in which
it is running.

If the binary-path ends in `.gz`, the binary will be decompressed.

#### Extension Interface

Test Extension binaries must implement a well-defined interface through which information about the
tests the binary offers can be discovered and executed. This interface includes several command line
verbs the binary must implement, arguments those verbs must support, and output formats the binary
must adhere to.

Running an extension binary will output the following help text for the initial version of the interface.

```
info        - Output test contribution extension version and metadata.
list        - Output tests supported by this extension.
run-test    - Run one or more tests and output results.
run-suite   - Runs tests associated with suites supplied by this extension.
run-monitor - Runs one or more test monitors.
config      - Component configuration management.
update      - Update git metadata for extension.
```

##### Info - Extension Metadata

The extension interface will evolve over time. For this reason, the `info` verb
will output information about which version of the interface the binary supports. The origin
framework will support some level of skew in the interface versions and invoke the binaries
with verbs & arguments consistent with the interface version they support.

Info also provides information to origin about how to construct new or contribute new
test suites.

```
$ extension-binary info
  --component    "default" or component name 
  
Exposed components:
- openshift:payload:hyperkube  
```

Annotated example `info` output is provided below.

```python
{
    # The extension interface version the binary implements.
    # The details of the interface and version will usually be
    # unnecessary for a contributor to understand as they will
    # vendor in the majority of the implementation from the
    # "Test Extension Support Module" explained later.
    "apiVersion": "1.0",

    # "source" contains git information from which this binary was
    # compiled
    "source": {
        "commit": "fceca0496512cad1fcadf1fc67674bff5c6b7b83",
        "build_date": "2024-10-09T13:10:20Z",
        "git_tree_state": "dirty"
    },

    # Aspects of the environment that the only extension 
    # can determine (e.g. the version of its target 
    # component).
    "environment": {

        # Relevant versions of software detected on the system under
        # test. origin will combine (and dedupe) the versions reported 
        # by in the aggregated test format output, so an extension
        # need only identify the versions of the software
        # it is testing (e.g. an OLM operator need not
        # report the version of OpenShift) or those relevant
        # to its test results that other extensions would might not
        # be reporting (e.g. an OLM dependentcy).
        "versions": [
            {
                "component": {
                    "product": "openshift",
                    "type": "payload",
                    "name": "hyperkube"
                },

                "version": "4.18.0",

                # A sha256 pullspec for an image
                # or a git URL to a specific commit. Something that
                # should identify the content of the 
                # software under test.
                "source": {
                    "type": "git",
                    "commit": "...",
                    "url": "github.com/..."
                },

                # If the component was upgraded from known versions.
                "from": [
                    {
                        "name": "...",
                        "source": "..."
                    }
                ]
            }
        ],
    },

    # A single extension can carry tests for multiple
    # components. However, each invocation of the binary
    # can only represent and act on information relative
    # to a single component.
    # If a binary carries tests for multiple components,
    # it must register itself multiple times, with
    # different pre-verb arguments which openshift-tests
    # will provide verbatim with every invocation; e.g.
    # if the binary registers with "--component hyperkube",
    # verb invocations will be as follows:
    #    extension-binary --component hyperkube info
    #    extension-binary --component hyperkube list ...
    "component": {
        "product": "openshift",
        "type": "payload",
        "name": "hyperkube"
    },

    "originalComponent": {
        # This can be populated to maintain continuity
        # with historical tests after a component rename.
    },
    
    "configurations": [
        {
            "profile": "common-mode-1",
            "description": "Enables a common, non-default mode of operation where ....",
            "application": {
                # If applying the configuration implies a disruption, inform 
                # openshift-tests, so that it can be accounted for in overall
                # disruption reporting.
                "disruption": "1m",
                # If, after applying the component configuration, openshift-tests
                # should await cluster steady state before running configuration
                # specific tests. If not specified, openshift-tests assumes
                # the tests can be run immediately after applying the configuration
                # profile.
                "await": "steady-state"
            },

            "resources": {

                # openshift-tests will only apply a single configuration
                # profile to a component at a given time.
                # However, inter-component conflicts might still occur.
                # Configurations can request to be run in isolation
                # from other component configurations. 
                # If two component configurations have a conflict name in common, 
                # test planning will ensure that there is no attempt to 
                # apply those profiles simultaneously.
                "isolation": {
                    "conflict": [
                        "gpu",
                    ]
                },

                # Tests that require significant resources can identify
                # that fact so that the planning algorithm will not
                # overdraw on allocatable pod resources for a set of
                # tests run in parallel.
                "memory": "1Gi",

                # If a duration can be roughly predicted, inclusion of this
                # information may improve the test execution 
                # planning algorithm performance.  
                "duration": "2s",

                # Timeout information may also support efficient
                # planning.
                "timeout": "16s",
            },

        }
    ],

    # Additional suites the extension wants to advertise / participate in.
    "suites": [
        {
            # Here, the extension is advertising a new suite, unknown
            # to the origin framework until discovered through the
            # binary. 
            "name": "fips/conformance",

            "parents": [

                # This suite can be run separately or will be included
                # automatically in other suites known to origin.
                # Here, the extension is informing origin that the suite
                # is a sub-suite of openshift/conformance. All information
                # about a suite is additive across extension binaries.
                # For example, multiple binaries can advertise the same
                # suite -- if they advertise different parents, then
                # origin will treat the suite as a subset of each 
                # identified parent.
                "openshift/conformance"
            ],

            # Test cases can be advertised from across a number
            # of extension binaries. The following cel expressions
            # are OR'd together. If they select a test advertised
            # from another binary, they will be considered part of
            # this advertised suite.
            # Test tags, names, and other attributes will be
            # described later.
            "qualifiers": [
                "(test.tags.suite==\"fips\" || test.name.contains(\"fips\")) && !test.labels.has(\"Disruptive\")",
                # Normally, CEL expressions evaluate against ALL test
                # specs discovered by origin, regardless of the extension
                # that advertised them. This makes it easy for 
                # a suite defined in one extension to "absorb" tests 
                # specified by another extension. To limit the expression
                # to tests in only certain extensions, you can filter on "source"
                "source = \"openshift:payload:hyperkube\" && test.name.contains(\"FIPS\"))"
            ]
        }
    ],
}
```

##### List Tests - Extension Test Listing

The "list tests" action will output information about tests exposed by the extension
for different environments. A test "environment" describes the attributes of
cluster under test (e.g. the cloud provider, the topology, etc).

The implementation should eliminate tests that are not appropriate for the
specified environment.

The arguments accepted by the verb may vary over time and will be
defined by the extension interface version implemented by the binary.

**Version 1**

```
$ extension-binary list tests 
Environment Information:
  --component     "default" or "<product>:<type>:<component>"
  --platform      The hardware or cloud platform ("aws", "gcp", "metal", ...).
  --network       The network of the target cluster ("ovn", "sdn"). 
  --upgrade       The upgrade that was performed prior to the test run ("micro", "minor").
  --topology      The target cluster topology ("ha", "microshift", ...).
  --architecture  The CPU architecture of the target cluster ("amd64", "arm64").
  --installer     The installer used to create the cluster ("ipi", "upi", "assisted", ...).
  --config        The component configuration to assume is active on the cluster.
  --version       "major.minor" version of target cluster. 

Optional/Devel:
  --suite         List only tests matched by specified suite's qualifiers.
```

The invocation will not be provided a kubeconfig and must output tests based solely
on environment information. If no environment arguments are provided, all tests must be
listed.

Listings are formatted as JSONL with one test description per line. A full listing contains
zero or more test description objects. The following example shows an abbreviated JSONL listing
for three tests.

```python
{"name": "T1", }
{"name": "T2", }
{"name": "T3", }
```

A test description object contains a number of attributes to inform `openshift-tests`
discovery and planning. Note that the following annotated example would be encoded into a single
line of output in actual listing output.

```python
{

    # Base, human-readable test name. 
    "name": "openssl version compliance",

    # If a test name is updated at any time in the future,
    # originalName must report the original name of the 
    # testing logic. This allows component readiness
    # to display the human-readable version of the test
    # name while considering test runs across name changes.
    "originalName": "security version compliance",

    # Labels are text strings will can be used like 
    # Ginkgo labels to group and run tests. Until labels
    # are part of bigquery data, labels will be appended
    # to test names with "[label]" so that name based
    # suite construction will work. 
    "labels": [
        "sig-..."
    ],

    # Tags are key=value pairs that can be used to 
    # further classify tests.
    "tags": {
        "key": "value"
    },

    "resources": {
        # Tests can request to be run in isolation
        # from others at different levels:
        # "instance" - avoids being called along with a conflicting
        #              test in a single run-test invocation.
        # "exec"     - avoids being called at the same time in a
        #              across all parallel extension binary invocations.
        # "bucket"   - avoids being called in the same planning bucket.
        # If two tests have a conflict name in common, the isolation
        # mode will apply. "*" is a special conflict name will 
        # ensures complete isolation in the requested mode (e.g.
        # guaranteeing the test will be the only one to
        # a given invocation of run-test if mode=instance).
        "isolation": {
            "mode": "exec",
            "conflict": [
                "gpu",
            ]
        },

        # Tests that require significant resources can identify
        # that fact so that the planning algorithm will not
        # overdraw on allocatable pod resources for a set of
        # tests run in parallel.
        "memory": "1Gi",

        # If a duration can be roughly predicted, inclusion of this
        # information may improve the test execution 
        # planning algorithm performance.  
        "duration": "2s",

        # Timeout information may also support efficient
        # planning.
        "timeout": "16s",
    },

    # Tests can be identified as "informing" or "blocking".
    # Informing tests will not negatively impact default views
    # in Component Readiness.
    # However, policies may be established in the future that 
    # cause tests that remain informing too long to be treated
    # as if they are production.
    "lifecycle": "informing",

}
```

##### Run-Test - Running Extension Tests

The `run-test` verb is used to cause the extension binary to actually run
discovered tests. `run-test` accepts test environment arguments as discussed
in the "list" verb section (in order to reduce fact discovery time needed
for each test run) and one or more test names to invoke.

```
$ extension-binary run-test 
  --component     "default" or "<product>:<type>:<component>"
  --platform      The hardware or cloud platform ("aws", "gcp", "metal", ...).
  ...other environment arguments...
  --config        The component configuration profile to assume is active on the cluster.
  --name | -n     Test name to invoke (-n can be specified multiple times).
  --list          Filename or "--" for stdin of tests to invoke from 'list' output.
```

`run-test` will run the specified tests in parallel. To prevent test cases developing a dependence 
on one another, `run-test` must make no assumption about the number or 
composition of tests it is asked to run. `origin` will also invoke `run-test`
with sets of tests consistent with the test environment and isolation requirements defined 
by the "list" verb). Non-origin users (e.g. developers running the extension directly)
may not take isolation requirements into account. To prevent conflicts in this situation,
in-process mutexes can be utilized OR tests can simply fail (reporting that a conflict
was inevitable).

`openshift-tests` will randomize tests into multiple different parallel
executions of the extension binary in order to amortize binary initialization
costs.  

Standard output from the invocation will be serialized into JSON with indentation
for human readability. Origin will call run-test with `-o jsonl` which will output
the results as they complete in JSONL.

Standard error includes the live output from ginkgo or your test framework as it runs.

JSONL format:

```python
{"name": "T1", "result": "success", }
{"name": "T1", "result": "success", }
{"name": "T1", "result": "success", }
```

A single test result will contain the following information, encoded on a single line:

```python
{
    "name": "test name",

    # RFC-3339 start and stop time in UTC with millisecond precision.
    "startTime": "2026-01-02T15:04:05.000Z",
    "endTime": "2026-01-02T15:04:06.840Z",

    # The outcome of the test; pass, fail, skip, timeout.
    "result": "pass",
    "output": "standard output of the test",
    "error": "standard error of the test",

    # Lifecycle of the test - whether it's blocking, informing, or experimental
    "lifecycle": "blocking",

    # Human-readable messages to further explain 
    # skips / timeouts / etc. It can also be used to provide 
    # contemporaneous information about failures that may
    # not be easily returned by must-gather. For larger artifacts
    # (greater than 10K), write them to $EXTENSION_ARTIFACTS_DIR
    "details": [
        {
            "name": "api-timeout",
            # "value" is anything that can be marshalled as JSON.
            "value": {"err": "I/O timeout connecting to port 8787"},
        },
    ],
    
    # In the case of a failure, a extension can identify an open
    # issue explaining the failure. Component Readiness will
    # optionally take this into account when notifying teams
    # or displaying information about a test's regressions.
    # Issues must be active at the time the failure is 
    # recorded for Component Readiness to consider a valid
    # explanation. This allows teams to redirect attention to
    # an appropriate component when their component is not
    # the root cause.
    "triage": [
        {
            "type": "jira",
            "issue": "TRT-5555",
            "description": "optional human readable brief"
        }
    ]
}
```

Large artifacts required to interpret test results can be written
to the path specified by `EXTENSION_ARTIFACTS_DIR` environment variable.
`origin` will pass the same directory to every invocation of the extension
for a given component -- so care should be taken to not overwrite files
from previous invocations.

The filename format: `<utc millis since the epoch>-<pid>-<artifact-name>.<artifact-extension>`
by the test extension framework when it is asked to store an artifact.


##### Run-Suite - Running Tests in Local Suites

When developing an extension, it may be useful to be able to run a suite
advertised by an extension directly. This can be accomplished using the built-in
`run-suite` command:

```
$ ./extension run-suite my-suite --platform aws ...
```

This can also be accomplished using a combination of `list` and `run-test`.
`run-test` can also take a list of tests to run on stdin when piped from another tool,
for example:

```
$ ./extension list -o names --suite my-suite --platform aws ... | ./example-tests run-test --platform aws ...
```

Note that `run-test` will blindly execute tests in the list as quickly as possible,
in parallel, without consideration for system resources or parallelism constraints
`list` may advertise. Resource constraints will only be honored by `origin` driven
test orchestration -- it will choreograph invocations of `run-test` consistent with
those constraints.

##### List Monitors - List which monitoring tests are available

Monitors are processes that will be run by origin for the duration of
test execution. They are similar to tests in that they can write
artifacts and report back tests results. They differ in that they
cannot have any conflicts/isolations (they must be able to run
during all testing).

Similar to "list tests", "list monitors" will receive the environment
information via command line arguments. It should not list monitors 
inappropriate for the target environment.

Like tests, monitors are listed by origin in JSONL. A single monitor
entry includes:

```python
{
    # The name of the monitor (will be passed to run-monitor) 
    "name": "fips-endpoints",
    "description": "optional description",

    # Monitors can specify whether they should be run in order to 
    # save resources when they have no value. If they select any tests
    # origin identifies execution, origin will run the monitor. If
    # no qualifiers are specified, the monitor will always be run.
    "qualifiers": [
        "source = \"openshift:payload:hyperkube\" && test.name.contains(\"FIPS\"))"
    ]
}
```

##### Run-Monitor - Monitoring Cluster during Test Run

The `run-monitor` will start a monitor identified by `list monitors`. A monitor should 
stay running until it receives SIGINT from origin. After receiving SIGINT, 
it will be given a 30-second grace period before receiving a SIGKILL.

```
$ ./extension-binary run-monitor 
  --component     "default" or "<product>:<type>:<component>"
  --platform      The hardware or cloud platform ("aws", "gcp", "metal", ...).
  ...other environment arguments except config...
  --name | -n     Test name of the monitor to run (-n can be specified multiple times).
```

`run-monitor` will receive the same environment parameters as `run-test` --
exception `--config` which vary during the course of execution -- and
will output to the same formats (e.g. JSONL test results). `run-monitor` should
always output at least one test result. A test result in the stdout
stream must be called 'Monitor: <monitor name>' for each monitor that
was run with the invocation and reflect the success or failure of the monitor.

Failure to include this test result will result in `origin` creating it
synthetically and reporting it as a failure.

##### Config - Component Configuration Testing

A component can advertise that it wants to be exercised in multiple different configuration.
`openshift-tests` will plan an efficient method of testing those configurations and
ensure tests appropriate to that configuration are run.

The `config` verb will not return until the extension has watched the component
fully apply the configuration (or it should timeout with an error).

```
$ extension-binary config
  --component                   "default" or "<product>:<type>:<component>"
  --profile <profile name>      The configuration profile to apply or "default".
```

The component configuration is considered part of the Environment and thus passed
in to `list` in order to determine appropriate tests for the configuration.

Component authors may choose to reduce the number of tests run for non-default
configuration profiles, focusing only on tests likeliest to fail based on the
configuration change, in order to reduce overall execution time.

`openshift-tests` will plan an efficient testing strategy based on the
extension binaries it discovers, the configuration profiles they support,
and the tests that need to run in each configuration. It will:

1. Discover extension binaries.
2. Discover extension binary metadata (`info` verb), including configuration profiles.
3. Discover tests that should be run for each configuration profile (`list` verb with `--config <profile>`, for each
   profile).
4. Plan test execution by grouping configuration profiles into buckets.

After `openshift-tests` plans test orchestration, it will proceed to execute tests in
each configuration bucket. For a given bucket, `openshift-tests`
will:

1. Iterate through all involved components and configure them with non-conflicting configuration profiles (`config`
   verb).
2. Await a high-level cluster steady state (cluster operators available, machines config up-to-date) if any
   configuration profile requires it.
3. Run tests within the bucket with non-conflicting, test-level parallelization.

Between buckets, the component may be asked to set its required `default` profile in order
to return to its install-time configuration.

#### Update - Metadata Validation

Component owners will be responsible for implementing the extension interface. To prevent common mistakes
and ensure conformance with the evolution of their implementation, `make` (or similar build system)
must run `<extension binary> update [--component product:type:component] [--basedir <basedir>]` after the extension
binary is built.

The `update` verb will create or update files under `hack/.openshift-tests-exension/product/type/component/*`, by
default
(basedir defaults to `hack/.openshift-tests-extension`).
If an incompatible change is introduced from the prior invocation of `update` (e.g. changing
a test name without preserving the original), `update` will raise an error
which the component owner must correct before committing their change in git.

The content of the files stored under `.openshift-tests-extension` is subject to change and wholly
defined by the `github.com/openshift-eng/openshift-tests-extension` module. Individual component
owners should not modify files under this path except through invocations of `update`.

#### Extension Implementation

github.com/openshift-eng/openshift-tests-extension will make contributing tests as simple
as possible. An example of what implementing the extension interface with this module
is provided below.

```golang
componentExtension := extensions.NewExtension("openshift", "payload", "hyperkube")

// Allow extensions to Walk tests and return them wrapped
// in a wrapper extensions.ExtensionTestSpec. We can offer these
// helper functions to ingest tests from different testing framework.
var ginkgoExtensionSpecs extensions.ExtensionTestSpecs

// In this case, they are asking us to wrap Ginkgo tests they assert follow our 
// historical conventions.
ginkgoExtensionSpecs, err = extensions.BuildExtensionTestSpecsFromOpenShiftGinkgoSuite()

// If we don't support their testing framework, they can wrap tests
// with lower level API. Here, they give us an implementation that builds
// a set of ExtensionTestSpecs. Each ExtensionTestSpec should have a `Run() 
// *ExtensionTestResult` function that runs the test and returns our result format to 
// us. This allows you to wrap essentially any test framework, or none at all.  
// Implementations of `Run()` must be thread safe.  You can use a mutex to prevent 
// parallel execution.
var customExtensionSpecs extensions.ExtensionTestSpecs
customExtensionSpecs, err = BuildExtensionTestSpecsFromCustomSource(...) 

// Once individually wrapped in ExtensionTestSpec and grouped into ExtensionTestSpecs
// handling the ginkgo vs custom tests becomes identical. ExtensionTestSpec (ETS) is where you
// can store all of the metadata that the enhancement calls for on a test-by-test basis.

// Label all ETS in this set as slow.. Common ones, custom ones, it doesn't matter.
// BuildExtensionTestSpecsFromGinkgoSuite may pre-populate tags using the regex [.*]
// on tests names, while the lower-level API may not make this assumption.
customExtensionSpecs.AddLabel(commonlabels.SLOW) 

// similar story with tags.
customExtensionSpecs.AddTag({"mytag": "myvalue"})  

// Here, OpenShift convention was used to determine which tests to run on AWS
// As a contrived example, let's say we want to run these same tests on GovCloud
awsTests := ginkgoExtensionSpecs.Select(extensions.PlatformSelector(platforms.AWS))
awsTests.AddPlatform(platforms.GovCloud)

// And so on with expressive grouping & metadata management. awsTests
// references the same objects as ginkgoExtensionSpecs, so changes in
// one are reflected immediately in both sets.
awsTests.Select(extensions.LabelSelector(platform.SLOW)).SetResource(resource.CPU, "5000m")

// Since this set of tests doesn't use the OpenShift naming conventions, they
// must constrain environmental parameters themselves.
customExtensionSpecs.Select(extensions.NameRegExSelector(".*cld/aws.*")).SetPlatforms(platforms.AWS)

// You can add hooks to execute BeforeAll, BeforeEach, AfterAll, AfterEach for 
// ExtensionTestSpecs.  These are sticky to that group individual specs, so you're still
// able to use different initialization code per spec set.
customExtensionSpecs.AddBeforeAll(func() {
    // perform custom extension initialization
})

ginkgoExtensionSpecs.AddBeforeAll(func() {
    // perform custom ginkgo initialization
})

// Since everything is an ExtensionTestSpec, you can mix sets.
allSpecsSet := customExtensionSpecs
allSpecsSet.Add(ginkgoExtensionSpecs)

allSpecsSet.AddAfterAll(func() {
    // perform clean up that applies to both sets
})

// The author can iterate through all tests and, for example,
// set OriginalName for each based on a configuration file
// or dictionary maintained in their repository.
allSpecsSet.Walk(func(*ExtensionTestSpec) {
    // do stuff
})

// Finally, some or all of the specs are added for the component. 
componentExtension.AddSpecs(allSpecsSet...)

customSuite := extension.Suite{
    Name: "my-custom-suite",
    Parents: []string{
        "openshift/conformance/parallel",
    },
    Qualifiers: []string{
        // CEL expression which selects appropriate tests
        "(test.tags.mytag==\"myvalue\"", 
    }
}

// AddGlobalSuite adds a suite whose qualifiers will extend to all test binaries
componentExtension.AddGlobalSuite(customSuite)

// AddSuite adds a suite whose qualifiers are automatically modified to filter on
// source = this component.
componentExtension.AddSuite(customSuite)

// The first component in the NewExtensionRegistry argument list
// will be selected by --component default, but also
// selectable with --component openshift:payload:hyperkube . 
// Most authors will be writing tests for one component, so origin
// omitting an option for --component, unless instructed otherwise, will "just 
// work".
componentRegistry := e.NewRegistry()
componentRegistry.Register(componentRegistry)

root := &cobra.Command{
    Long: "OpenShift Tests Extension Example",
}

// When using Cobra, this allows the main verb names to change without the author needing to
// enumerate them.
root.AddCommand(cmd.DefaultExtensionCommands(registry)...)
```

#### Test Result Aggregation

`openshift-tests` will store test results in the CI artifacts repository in a JSONL file.
The output will be similar to that of the `run-test` output, but each test result
will be expanded to include:

- Component
- Environment
- Testing Context

The goal here is to allow a single file to contain comprehensive information necessary for
a system like Component Readiness to analyze it without, for example, needing to draw information
from prowjob names.

```python
{
    # Human-readable test name
    "name": "openssl security compliance",

    # Original human-readable test name, if it has changed over time.
    # If not specified by `list`, it defaults to the current 
    # name.
    "originalName": "fips security compliance",

    # The Test ID openshift/origin has defined for this test based
    # on component and test metadata. It is meant to be unique
    # across all other components and consistent across time
    # (even if the human-readable name for a unit of test logic
    # is changed).
    "id": "openshift-payload-api-server-fips security compliance",

    "result": "pass",

    # ...other test result information...

    # Information necessary to fully qualify the 
    # test.
    "component": {
        "product": "openshift",
        "type": "payload",
        "component": "hyperkube",
    },

    "originalComponent": {
        # If not specified by `info`, this will default to the
        # current value. 
        "product": "openshift",
        "type": "payload",
        "component": "hyperkube",
    },
    
    "environment": {
        "platform": "aws",
        "architecture": "amd64",
        # ...others...

        # Configuration information is also included. 
        "configuration": {
            # The configuration id applied to the component
            # before the test was run.
            "component": "default",
        },

        # Aggregated version information from all extensions
        # as well as what origin can identify.
        "versions": [
            # Here, the environment involves a management cluster
            # and a hosted cluster. They have independent versions.
            {
                "component": {
                    "product": "openshift",
                    "type": "management",
                },

                "version": "4.18.0",

                "source": {
                    "type": "image",
                    "digest": "sha256:...",
                    "url": "registry.ci.openshift.org/..."
                },

                # If the component was upgraded from known versions.
                "from": [
                    {
                        "name": "...",
                        "source": "..."
                    }
                ]
            },
            {
                "component": {
                    "product": "openshift",
                    "type": "hosted",
                },

                "version": "4.17.0",

                "source": {
                    "type": "image",
                    "digest": "sha256:...",
                    "url": "registry.ci.openshift.org/..."
                },

                "from": [
                    {
                        "name": "...",
                        "source": "..."
                    }
                ]
            }
        ],
    },

    # Information about how the test was run which should
    # not alter the outcome of the test.
    "context": {
        # A small number of seeds can be used to randomize test
        # parallelization and bucketing. Analyzing seeds against
        # one another should highlight parallelization issues.
        # Using the same seed with the same tests should result
        # in identical planning and parallelization. 
        "seed": 15,

        # A sha256 of the tests IDs which were run. This, plus
        # seed should ensure identical execution pattern between
        # two runs.
        "testHash": "deadbeef",

        # In the future, we may run tests several times against
        # the same cluster in order to aggregate
        # test results more quickly. We may want to 
        # analyze results including this dimension if, for example,
        # we want to check whether the first run is as consistent
        # as the following N.
        "run": 0,

        "job": {
            # Name of the job which drove the test
            "name": "...",
            # Where the job information can be reviewed.
            "url": "..."
        },

        # Attributes of the host running the tests (not the 
        # system under test) or cloud account used. This can 
        # help us statistically analyze whether one system 
        # involved in running the testing is introducing 
        # failures that another is not (e.g. because it has 
        # a flakey network).
        "testDriver": {
            "cluster": "build01",
            "arch": "arm64",
            # Cloud account in which the ephemeral cluster was 
            # installed.
            "account": "34908092383"
        }
    }
}
```

### Risks and Mitigations

#### Binary Incompatibility

Copying a binary from a different image into a running pod exposes us to binary incompatibility.

##### CPU Architecture

An arm64 binary will not run in an amd64 based pod. `openshift-tests` will need to run on
a build farm node with a CPU architecture consistent with architecture of the cluster
payload under test.

#### Runtime Size / Speed

Statically linked binaries are large relative to dynamically linked binaries. As the number
of extension binaries increase, so will the required tmpfs space used by pods running the
tests as those binaries are stored locally within the pod.

To mitigate this risk, we need to be aware of it, overprovision test pods with respect to
the memory normal test execution requires, and also include a test failure that will
TRT attention if the pod is using > 80% of allocatable memory.

#### Image Size

Statically linked binaries are relatively large. As the number of test extension binaries
increases, it may lead to a noticeable increase in overall payload size.

#### Poor Extension Implementation

Providing a contract that must be implemented by test extensions without centralized
control & review of the implementation leaves space for contributions that violate the
expectations of that contract.

### Version Skew Strategy

The majority of test extension binaries will reside within the OpenShift payload under test, and thus cannot
skew. For non-payload components, test filtering, performed by the extension binary itself can take the current
platform version into account when determining appropriate tests to run.

## Alternatives

1. Run extension binaries from Pods/independent container images without extracting them to tmpfs.