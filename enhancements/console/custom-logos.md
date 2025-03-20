---
title: custom-logos
authors:
  - "@cajieh"
reviewers:
  - "@jhadvig "
  - "@spadgett"
  - "@everettraven"
  - "@JoelSpeed"
approvers:
  - "@spadgett"
api-approvers:
  - "@JoelSpeed"
creation-date: 2025-02-11
last-updated: 2025-02-11
---

# Custom Logos

## Summary

The OpenShift Container Platform (OCP) web console was upgraded to PatternFly 6. In PatternFly 6, the masthead color changes based on light or dark mode. As a result, a single custom logo may not be suitable for both themes.

This proposal is to add the ability to specify custom logos to support light and dark theme modes for the masthead and favicon in the Console UI.

## Background info

The OpenShift Container Platform (OCP) web console was upgraded to PatternFly 6. In PatternFly 6, the masthead color changes based on light or dark mode. As a result, a single custom logo may not be suitable for both theme modes. 

To address this, we need to allow users to add custom logos compatible with light and dark themes for both the masthead and favicon. This ensures that the logos are always visible and aesthetically pleasing, regardless of the theme mode.

The custom logos feature will enable users to specify different logos for the masthead and favicon based on the theme mode. This will involve exposing a new API endpoint to support custom logos for both light and dark themes.

## Motivation

The existing OKD and OpenShift logos are designed for a dark masthead background and include white text, making them ineffective in a light theme. To ensure logos remain visible and visually appealing in both light and dark themes, users need the ability to add custom logos for the masthead and favicon that adapt to the theme mode. To support this, a new API endpoint will be introduced, allowing users to specify different logos for light and dark themes.

### User Stories

* As an OpenShift web console user, I want to be able to add different custom logos for light and dark theme modes in the masthead and favicon.

### Goals

This feature will allow users to add custom logos for the masthead and favicon that are compatible with both light and dark themes in the OpenShift web console.

### Non-Goals


## Proposal

### API Design Details

The configuration for custom logos will include support for both masthead and favicon types with separate files for light and dark themes:

```yaml
customLogos:
  - type: Masthead
    themes:
      - type: Light
        file:
          key: logo-light.svg
          name: masthead-logo-light
      - type: Dark
        file:
          key: logo-dark.svg
          name: masthead-logo-dark
  - type: Favicon
    themes:
      - type: Light
        file:
          key: favicon-light.png
          name: favicon-logo-light
      - type: Dark
        file:
          key: favicon-dark.png
          name: favicon-logo-dark
```


### Workflow Description

├── spec
│   ├── customization
│       ├── customLogos
│           ├── type
│           ├── themes
│               ├── type
│               ├── file
│                   ├── key
│                   ├── name
└── ...

### API Extensions

None

### Risks and Mitigations

1. 1. Users could set both the `CustomLogoFile` and `CustomLogos` APIs, and the `CustomLogos` API configuration will take precedence over the old `CustomLogoFile` field.

2. Each of the Console supported themes can be configured individually by setting either the Light or Dark theme type, or by applying a default theme to all supported themes using the Default theme type. If the Default theme type is set along with a specific Dark or Light theme, the specific theme setting will override the default one.

3. Users might experience confusion with the introduction of new logo configuration options. The prevoious method represented by `CustomLogo` will be deprecated. Provide comprehensive documentation that will guide users through the transition. Include clear instructions about the changes and their benefits.

### Drawbacks

None

### Attributes Description

#### customLogos
- `type`: Enumerate which specifies the type of custom logo. Available custom logo types are `Masthead` and `Favicon`.
- `themes`: A list of themes for which the custom logo is defined.

#### themes
- `type`: Enumerate which specifies the type of theme. Available theme types are `Light`, `Dark` and `Default`.
- `file`: Contains the file details for the custom logo.

#### file
- `key`: The key or path to the custom logo file.
- `name`: The name of the ConfigMap containing the custom logo file.

## Test Plan

Provide tests as part of the console `CustomLogos` implementation and verify that it was shown in the UI. The following tests will be added:
 - Unit tests for API
 - Unit and E2E tests for console-operator
 - E2E tests for console


## Graduation Criteria

None


#### Dev Preview -> Tech Preview

None

### Tech Preview -> GA

None

### Dev Preview -> Tech Preview

None

#### Removing a deprecated feature

The current custom logo field in `customization.customLogo` is deprecated and will be removed in a future release. Users are encouraged to transition to the new custom logos configuration that supports light and dark modes for the masthead and favicon. The new custom logos feature also includes support for a default theme for all unspecified themes.

None

#### Failure Modes

None

### Removing a deprecated feature

None

## Upgrade / Downgrade Strategy

None

## Version Skew Strategy

None

## Operational Aspects of API Extensions

None

## Support Procedures

None

## Tracking Link

For more information, refer to the [OpenShift Documentation](https://docs.openshift.com/container-platform/4.17/web_console/customizing-the-web-console.html#adding-a-custom-logo_customizing-web-console).

TODO:  Update the URL to point to CustomLogos docs later on.

### Implementation Details/Notes/Constraints

None

### Topology Considerations

None

#### Hypershift / Hosted Control Planes

None

#### Standalone Clusters

None

#### Single-node Deployments or MicroShift

None

## Alternatives

None
