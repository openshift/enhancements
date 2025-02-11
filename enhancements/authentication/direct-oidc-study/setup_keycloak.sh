#!/usr/bin/env bash
set -e

# run from the cluster-authentication-operator repo
OPENSHIFT_KEEP_IDP=true WHAT=TestKeycloakAsOIDCPasswordGrantCheckAndGroupSync make run-e2e-test

KC_NS="$(oc get ns -l'e2e-test=openshift-authentication-operator' --no-headers | head -1 | cut -d' ' -f1)"
KC_URL="https://$(oc get route -n $KC_NS test-route --template='{{ .spec.host }}')/realms/master"

oc patch cm -n openshift-config-managed default-ingress-cert -p '{"metadata":{"namespace":"openshift-config"}}' --dry-run=client -o yaml | oc apply -f -
oc patch proxy cluster -p '{"spec":{"trustedCA":{"name":"default-ingress-cert"}}}' --type=merge

curl -k "${KC_URL}/.well-known/openid-configuration" > oauthMetadata
oc create configmap oauth-meta --from-file ./oauthMetadata -n openshift-config

oc patch authentication cluster -p '{"spec":{"oauthMetadata":{"name":"oauth-meta"},"type":"None"}}' --type=merge

oc patch kubeapiserver cluster -p '{"spec":{"unsupportedConfigOverrides":{"apiServerArguments":{"oidc-ca-file":["/etc/kubernetes/static-pod-certs/configmaps/trusted-ca-bundle/ca-bundle.crt"],"oidc-client-id":["admin-cli"], "oidc-issuer-url":["'"${KC_URL}"'"]}}}}' --type=merge

# this should get us a token
curl -k "$KC_URL/protocol/openid-connect/token" -d "grant_type=password" -d "client_id=admin-cli" -d "scope=openid" -d "username=admin" -d "password=password" | jq -r '.id_token'

