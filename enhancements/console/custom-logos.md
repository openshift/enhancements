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

The OpenShift Container Platform (OCP) web console is currently being upgraded to PatternFly 6. In PatternFly 6, the masthead color changes based on the mode (light or dark). Consequently, a single custom logo may not be suitable for both modes. 

Add the ability to specify custom logo features to support light and dark theme modes for the masthead and favicon.

## Background info

The OpenShift Container Platform (OCP) web console is currently being upgraded to PatternFly 6. In PatternFly 6, the masthead color changes based on the mode (light or dark). Consequently, a single custom logo may not be suitable for both modes.

To address this, we need to allow users to add custom logos for both the masthead and favicon that are compatible with light and dark themes. This ensures that the logos are always visible and aesthetically pleasing, regardless of the theme mode.

The custom logos feature will enable users to specify different logos for the masthead and favicon based on the theme mode. This will involve extending the API to support custom logos for both light and dark themes.

## Motivation

The OpenShift Container Platform (OCP) web console is currently being upgraded to PatternFly 6. In PatternFly 6, the masthead color changes based on the mode (light or dark). Consequently, a single custom logo may not be suitable for both modes. This is evident with the existing OKD and OpenShift logos, which assume a dark masthead background and include white text.

### User Stories

* As an OpenShift web console user, I want to be able to add custom logos for light and dark theme modes in the masthead and favicon.

### Goals

This feature should allow users to add custom logos for the masthead and favicon that are compatible with both light and dark theme modes in the OpenShift web console.

### Non-Goals


## Proposal

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

### Risks and Mitigations

1. Users could set both the `CustomLogoFile` and `CustomLogos` APIs. When both APIs are set. 

To mitigate this challenge, the `CustomLogos` API configuration will take precedence over the old `CustomLogoFile` field.

2. Handling different custom logos for light and dark themes may increase the complexity of the backend and UI logic. 

To mitigate this challenge, thoroughly test and validate the new logic, and implement fallback mechanisms to ensure logos are always visible if any are missing.

3. Users might experience confusion with the introduction of new logo configuration options, especially if they are familiar with the old method, which may soon be deprecated.  

To mitigate this challenge, provide comprehensive documentation and tools that guide users through the transition. Include clear instructions about the changes and their benefits.


### Drawbacks

N/A

## API Design Details

The configuration for custom logos will include support for both masthead and favicon types, with separate files for light and dark themes:

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
          key: favicon-light.ico
          name: favicon-logo-light
      - type: Dark
        file:
          key: favicon-dark.ico
          name: favicon-logo-dark
```

### Attributes Description

#### customLogos
- `type`: Enumerate which specifies the type of custom logo. Available custom logo types are `Masthead` and `Favicon`.
- `themes`: A list of themes for which the custom logo is defined.

#### themes
- `type`: Enumerate which specifies the type of theme. Available theme types are `Light` and `Dark`.
- `file`: Contains the file details for the custom logo.

#### file
- `key`: The key or path to the logo file.
- `name`: The name identifier for the custom logo.

### Test Plan

### Graduation Criteria

N/A


#### Dev Preview -> Tech Preview

N/A

#### Tech Preview -> GA

N/A

#### Removing a deprecated feature

The current custom logo feature is deprecated and will be removed at some point in the future. Users are encouraged to transition to the new custom logo configuration that supports light and dark theme modes for the masthead and favicon.

N/A

#### Failure Modes

N/A

## Alternatives

