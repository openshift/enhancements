---
title: Internationalization
authors:
  - "@ralpert"
reviewers:
  - TBD
  - "@spadgett"
  - "@christianvogt"
  - "@janwright73"
approvers:
  - TBD
  - "@spadgett"
  - "@christianvogt"
  - "@janwright73"
creation-date: 2020-08-28
last-updated: 2020-09-02
status: implementable
---

# Internationalization

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [x] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Our goal is to add internationalization to OpenShift's front-end (with some constraints) in order
to improve the user experience for a wider range of customers worldwide.
OpenShift does not currently have this functionality.
We have created a POC using the [react-i18next framework](https://react.i18next.com/), which allows for the marking
and translation of strings in the application, and [moment.js](https://momentjs.com/) for localizing dates and times.
We have also added a plugin, [i18next-pseudo](https://github.com/MattBoatman/i18next-pseudo), that generates
pseudotranslations, and have connected with the Red Hat globalization team for early-stage testing and to learn more about the translation process
used for other products at Red Hat. We want to implement localization more widely in the console.

## Motivation

We seek to improve the user experience (UX) for a wider global range of customers.
This change is important because it will allow us to meet the technical and cultural
needs of multiple locales. For example, it will allow us to reach customers who would prefer using OpenShift in a different
language (i.e. Chinese or Japanese) rather than English.

Using the i18next library, we will have a single source code for all languages of the product and it will also be easier to
leverage content strategists to author content and review copy consistency. All translatable strings in the front-end will be marked in the code and will be easy to export and modify.

### Goals

* Mark and extract to-be-translated strings in console
* Test components using the pseudotranslation plugin to make sure components function well with longer text lengths
* Work with the globalization team to obtain translations and perform localization testing for Chinese and Japanese
* Work with UX to drive copy consistency in the UI (confirmation messages, actions that are more than one word, pieces from CoreOS, and similar actions with different verbiage)

### Non-Goals

We are not able to translate all text in the application. Text located in non-Red-Hat-controlled development environments may not be accessible for translation.
This may include items such as events surfaced from Kuberenetes, alerts, and error messages displayed to the user or in logs.
In addition, operators that surface informational messages may not have translations available, and localization of logging messages at any level is not in scope.
Localization will not be included in the CLI at this time, and bidirectional (right-to-left) text such as Hebrew and Arabic are out of scope.

## Proposal


### User Stories

As an OpenShift user, I want to access to the web console information in my preferred language.

[react-i18next framework](https://react.i18next.com/) an internationalization framework for React based on [i18next](http://i18next.com/).
It provides standard internationalization features such as plurals, context, interpolation, and format,
as well as language detection, loading, caching, and extensions through additional plugins.

[moment.js](https://momentjs.com/) allows for parsing, validating, manipulating, and displaying dates and times in JavaScript.
One of its many features is displaying dates and times correctly for a given locale.

We have already [created a PR](https://github.com/openshift/console/pull/6194) integrating react-i18next and moment.js into OpenShift.
We used react-i18next for language detection and string internationalization, and moment.js for date/time internationalization.
We have also been working with the Red Hat Globalization team to test out the hand-off process. In addition, we added a plugin,
[i18next-pseudo](https://github.com/MattBoatman/i18next-pseudo), that generates pseudotranslations so that we can identify
and fix components that can't handle longer text lengths and write integration tests to ensure good code coverage.
This plugin is enabled for the "en" locale using a simple query parameter - `?pseudolocalization=true&lng=en`.

We will need to mark all hard-coded strings with react-i18next so they can be extracted using [i18next-parser](https://github.com/i18next/i18next-parser) and handed off to the Globalization team.
This is not complex work but will involve many small code changes.
[A rough string search](https://gist.github.com/jschuler/a404aed1ea774383cc2decd57d3135c6)
of the frontend console code found about 9,000 strings, but many will not be translated (such as Kubernetes resource names).
We will need to identify a prioritized list of UIs in OpenShift for localization.

We will define a location under each of the packages folders for static plugins to keep their i18n files and treat
the console as a single product for translation until we migrate to dynamic plugins.

We will also need to do testing, coordinate with the Red Hat Globalization team on testing and translation,
and collaborate with console plugins on a localization strategy for releases. Finally, a product testing window will need to be defined.

### Implementation Details/Notes/Constraints [optional]

Localization will affect multiple teams within the console development space.
This will affect Dev console, Admin console, and other packages like CNV.
The solution must account for the plugin contribution model and enable dynamic extensions to provide localization as necessary.

We plan to break translations up into [multiple translation files](https://react.i18next.com/guides/multiple-translation-files), at least one per package.
This is expected to include all information we can localize in the console, potentially broken over multiple phases.
As mentioned earlier in this document, localization of all text within the system is not possible. In addition, some terms are Kubernetes terms and would not be translated.

Some components may need to be updated to accommodate longer text lengths.

The font system seems to handle Japanese and Chinese double-byte characters well.

We have planned a story with UXD to add a language switching menu so the user can opt out of the language setting provided by the browser.

The desired fallback experience if not all pieces/parts of the UI are translated for a given language will be that the text will be displayed in English.

We will also modify our existing file conversion tools to handle translation files spread across multiple packages so we can easily export/import translation files.

Translation of the login page will be handled by [oauth-server](https://github.com/openshift/oauth-server), which will check for request `Accept-Language` header. If the header is missing in the request, `oauth-server` will default to English language for localization.

[oauth-server](https://github.com/openshift/oauth-server) contains the default OKD login templates. Custom templates are stored in the [oauth-templates](https://github.com/openshift/oauth-templates) repository and will also need to be updated for the other brands like the OCP and OpenShift Dedicated login pages.
In case of any changes in these repositories, Console-Team will be responsible for syncing those changes between them.

Since there are only a small number of strings that rarely change, the localized strings for each of the supported languages will be directly hardcoded into the `oauth-server`, so we don't have to load them during runtime.
These strings are to be included in the console repository and the i18next-parser will add them into the files sent to the Globalization team for translation. Afterwards the console team will contribute them back to the [oauth-server](https://github.com/openshift/oauth-server).


### Risks and Mitigations

**Risk**: All text is not translated.

**Mitigation**: Work with the UXD team on a design strategy.

**Risk**: Translations are incorrect or don't work as expected.

**Mitigation**: We will work with the Red Hat Globalization team to thoroughly test the application and obtain culturally sensitive and accurate translations.
We will also write integration tests to ensure that the i18n-next translation framework is working as expected.

**Risk**: Components may not be able to accommodate longer text lengths seen in other languages.

**Mitigation**: We have added a plugin that generates pseudotranslations, which will allow us to test components against longer text lengths and make fixes as needed.

## Design Details

### Open Questions [optional]

* What will the size be for ongoing maintenance, support, and additional testing of localizations?
* What capacity does the existing localization team have for additional product support?
* Would every release of OpenShift be localized?

### Test Plan

We will have unit and integration tests. We will also work with the Globalization team to do localization testing.

### Graduation Criteria

Initially we will just mark strings internally. Translations and testing will be added once marking is complete.

#### Dev Preview -> Tech Preview

None

#### Tech Preview -> GA

None

#### Removing a deprecated feature

None

### Upgrade / Downgrade Strategy

We will need to make sure that new components with hard-coded strings are marked for translation.

### Version Skew Strategy

None


## Implementation History

* 2020-08-03 - [PR against console](https://github.com/openshift/console/pull/6194) opened
* 2020-04-23 - [UXD exploration of i18next](https://github.com/jschuler/console/pull/1) opened
* 2020-04-13 - [UXD analysis and comparison of libraries](https://github.com/patternfly/patternfly-react/issues/3952#issuecomment-613645855) posted
* 2021-03-25 - [PR against oauth-server](https://github.com/openshift/oauth-server/pull/71) opened

## Drawbacks

We will need to maintain accurate translations for every language we support and ensure that all new text in the front-end is considered for localization going forward. Testing and maintainence will also be a concern.

## Alternatives

UXD [did an analysis](https://github.com/patternfly/patternfly-react/issues/3952) of two internationalization frameworks, [react-intl](https://formatjs.io/docs/getting-started/installation/) and [react-i18next](https://react.i18next.com/), in April.

This was used to inform our direction. react-i18next was ranked better overall - it was found to have better documentation, and more active maintainers, as well as additional features that react-intl lacked.
We will make heavy use of these features, such as marked string extraction and splitting translations across multiple files.
