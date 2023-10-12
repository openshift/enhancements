# Getting a Keycloak instance

The cluster-authentication-operator tests allow you to create and persist a Keycloak instance. Run the `setup_keycloak.sh`
(the other file in this gist) bash script from your local copy of the cluster-authentication-operator repository.

It might be better to run it with the patch from https://github.com/openshift/cluster-authentication-operator/pull/609
in case Keycloak pod is shut down for some reason. Note that the changes you're doing to your Keycloak instance are not
persisted and so you might need to redo some of them in this case.

# Command line interface - oc

## Initial config

* oc login is broken - the "openshift-challenging-client" is missing:
  * Note that the OIDC provider is running **in the very same OpenShift cluster** behind a route. If it were running separately, we would likely get issues with the provider's certificate.

```
I0515 16:37:20.986177   66477 loader.go:372] Config loaded from file:  /home/slaznick/random_kubeconfig
I0515 16:37:20.986381   66477 round_trippers.go:466] curl -v -XHEAD  'https://api.sl-bd.group-b.devcluster.openshift.com:6443/'
I0515 16:37:20.987348   66477 round_trippers.go:495] HTTP Trace: DNS Lookup for api.sl-bd.group-b.devcluster.openshift.com resolved to [{52.7.27.156 } {34.238.77.130 } {35.168.253.93 } {107.23.124.234 } {52.205.180.113 }]
I0515 16:37:21.085613   66477 round_trippers.go:510] HTTP Trace: Dial to tcp:52.7.27.156:6443 succeed
I0515 16:37:21.285965   66477 round_trippers.go:553] HEAD https://api.sl-bd.group-b.devcluster.openshift.com:6443/ 403 Forbidden in 299 milliseconds
I0515 16:37:21.286044   66477 round_trippers.go:570] HTTP Statistics: DNSLookup 0 ms Dial 98 ms TLSHandshake 101 ms ServerProcessing 97 ms Duration 299 ms
I0515 16:37:21.286081   66477 round_trippers.go:577] Response Headers:
I0515 16:37:21.286117   66477 round_trippers.go:580]     X-Content-Type-Options: nosniff
I0515 16:37:21.286149   66477 round_trippers.go:580]     X-Kubernetes-Pf-Flowschema-Uid: 8db4d3d4-4c84-4a0c-9fc3-0f7e736b0bf4
I0515 16:37:21.286181   66477 round_trippers.go:580]     X-Kubernetes-Pf-Prioritylevel-Uid: e20e4cbc-8ab1-4280-8053-cdbd773f31b8
I0515 16:37:21.286213   66477 round_trippers.go:580]     Content-Length: 186
I0515 16:37:21.286243   66477 round_trippers.go:580]     Content-Type: application/json
I0515 16:37:21.286273   66477 round_trippers.go:580]     Strict-Transport-Security: max-age=31536000; includeSubDomains; preload
I0515 16:37:21.286305   66477 round_trippers.go:580]     Date: Mon, 15 May 2023 14:37:21 GMT
I0515 16:37:21.286337   66477 round_trippers.go:580]     Audit-Id: 4a4ee046-bf8a-4dab-bc17-21a920692670
I0515 16:37:21.286368   66477 round_trippers.go:580]     Cache-Control: no-cache, private
I0515 16:37:21.286411   66477 request_token.go:93] GSSAPI Enabled
I0515 16:37:21.286531   66477 round_trippers.go:466] curl -v -XGET  -H "X-Csrf-Token: 1" 'https://api.sl-bd.group-b.devcluster.openshift.com:6443/.well-known/oauth-authorization-server'
I0515 16:37:21.384583   66477 round_trippers.go:553] GET https://api.sl-bd.group-b.devcluster.openshift.com:6443/.well-known/oauth-authorization-server 200 OK in 97 milliseconds
I0515 16:37:21.384669   66477 round_trippers.go:570] HTTP Statistics: GetConnection 0 ms ServerProcessing 97 ms Duration 97 ms
I0515 16:37:21.384698   66477 round_trippers.go:577] Response Headers:
I0515 16:37:21.384735   66477 round_trippers.go:580]     Cache-Control: no-cache, private
I0515 16:37:21.384764   66477 round_trippers.go:580]     Content-Type: application/json
I0515 16:37:21.384792   66477 round_trippers.go:580]     Strict-Transport-Security: max-age=31536000; includeSubDomains; preload
I0515 16:37:21.384820   66477 round_trippers.go:580]     X-Kubernetes-Pf-Flowschema-Uid: 8db4d3d4-4c84-4a0c-9fc3-0f7e736b0bf4
I0515 16:37:21.384847   66477 round_trippers.go:580]     X-Kubernetes-Pf-Prioritylevel-Uid: e20e4cbc-8ab1-4280-8053-cdbd773f31b8
I0515 16:37:21.384877   66477 round_trippers.go:580]     Date: Mon, 15 May 2023 14:37:21 GMT
I0515 16:37:21.384903   66477 round_trippers.go:580]     Audit-Id: 3e15d235-133c-4741-b0fd-bf2819f0eb65
I0515 16:37:21.750557   66477 request_token.go:467] falling back to kubeconfig CA due to possible x509 error: x509: certificate signed by unknown authority
```
The above are the common, uninteresting bits that will be omitted from next logs.

Below we can see that the client found the proper auth URL and tries to reach it with `client_id=openshift-challenging-client` and fails with `400`:

```
I0515 16:37:21.750616   66477 round_trippers.go:466] curl -v -XGET  -H "X-Csrf-Token: 1" 'https://test-route-e2e-test-authentication-operator-gdw2f.apps.sl-bd.group-b.devcluster.openshift.com/realms/master/protocol/openid-connect/auth?client_id=openshift-challenging-client&code_challenge=SOQNg9806C9_GYWp1K_Thoiz6KQLgzXQST7iTquyRc8&code_challenge_method=S256&redirect_uri=https%3A%2F%2Ftest-route-e2e-test-authentication-operator-gdw2f.apps.sl-bd.group-b.devcluster.openshift.com%2Frealms%2Fmaster%2Foauth%2Ftoken%2Fimplicit&response_type=code'
I0515 16:37:21.751027   66477 round_trippers.go:495] HTTP Trace: DNS Lookup for test-route-e2e-test-authentication-operator-gdw2f.apps.sl-bd.group-b.devcluster.openshift.com resolved to [{107.22.212.24 } {54.166.125.229 }]
I0515 16:37:21.849199   66477 round_trippers.go:510] HTTP Trace: Dial to tcp:107.22.212.24:443 succeed
I0515 16:37:22.072343   66477 round_trippers.go:553] GET https://test-route-e2e-test-authentication-operator-gdw2f.apps.sl-bd.group-b.devcluster.openshift.com/realms/master/protocol/openid-connect/auth?client_id=openshift-challenging-client&code_challenge=SOQNg9806C9_GYWp1K_Thoiz6KQLgzXQST7iTquyRc8&code_challenge_method=S256&redirect_uri=https%3A%2F%2Ftest-route-e2e-test-authentication-operator-gdw2f.apps.sl-bd.group-b.devcluster.openshift.com%2Frealms%2Fmaster%2Foauth%2Ftoken%2Fimplicit&response_type=code 400 Bad Request in 321 milliseconds
I0515 16:37:22.072428   66477 round_trippers.go:570] HTTP Statistics: DNSLookup 0 ms Dial 98 ms TLSHandshake 102 ms ServerProcessing 120 ms Duration 321 ms
I0515 16:37:22.072459   66477 round_trippers.go:577] Response Headers:
I0515 16:37:22.072498   66477 round_trippers.go:580]     X-Frame-Options: SAMEORIGIN
I0515 16:37:22.072525   66477 round_trippers.go:580]     Content-Security-Policy: frame-src 'self'; frame-ancestors 'self'; object-src 'none';
I0515 16:37:22.072553   66477 round_trippers.go:580]     Content-Language: en
I0515 16:37:22.072579   66477 round_trippers.go:580]     Set-Cookie: 3c9e6724823ac7ad9a5d50942d705e4c=39f2e9b0235c633edc00cae002808130; path=/; HttpOnly; Secure; SameSite=None
I0515 16:37:22.072610   66477 round_trippers.go:580]     Content-Length: 1762
I0515 16:37:22.072636   66477 round_trippers.go:580]     Strict-Transport-Security: max-age=31536000; includeSubDomains
I0515 16:37:22.072676   66477 round_trippers.go:580]     X-Xss-Protection: 1; mode=block
I0515 16:37:22.072722   66477 round_trippers.go:580]     Referrer-Policy: no-referrer
I0515 16:37:22.072765   66477 round_trippers.go:580]     X-Content-Type-Options: nosniff
I0515 16:37:22.072795   66477 round_trippers.go:580]     X-Robots-Tag: none
I0515 16:37:22.072831   66477 round_trippers.go:580]     Content-Type: text/html;charset=utf-8
I0515 16:37:22.073892   66477 round_trippers.go:466] curl -v -XGET  -H "Accept: application/json, */*" -H "User-Agent: oc/v4.2.0 (linux/amd64) kubernetes/6e9a161" 'https://api.sl-bd.group-b.devcluster.openshift.com:6443/api/v1/namespaces/openshift/configmaps/motd'
I0515 16:37:22.172151   66477 round_trippers.go:553] GET https://api.sl-bd.group-b.devcluster.openshift.com:6443/api/v1/namespaces/openshift/configmaps/motd 403 Forbidden in 98 milliseconds
I0515 16:37:22.172240   66477 round_trippers.go:570] HTTP Statistics: GetConnection 0 ms ServerProcessing 97 ms Duration 98 ms
I0515 16:37:22.172283   66477 round_trippers.go:577] Response Headers:
I0515 16:37:22.172320   66477 round_trippers.go:580]     Cache-Control: no-cache, private
I0515 16:37:22.172348   66477 round_trippers.go:580]     X-Kubernetes-Pf-Prioritylevel-Uid: e20e4cbc-8ab1-4280-8053-cdbd773f31b8
I0515 16:37:22.172389   66477 round_trippers.go:580]     X-Content-Type-Options: nosniff
I0515 16:37:22.172435   66477 round_trippers.go:580]     X-Kubernetes-Pf-Flowschema-Uid: 8db4d3d4-4c84-4a0c-9fc3-0f7e736b0bf4
I0515 16:37:22.172482   66477 round_trippers.go:580]     Content-Length: 303
I0515 16:37:22.172521   66477 round_trippers.go:580]     Date: Mon, 15 May 2023 14:37:22 GMT
I0515 16:37:22.172566   66477 round_trippers.go:580]     Audit-Id: eabfc6c1-3ee1-4190-afce-0fe8d2c030db
I0515 16:37:22.172598   66477 round_trippers.go:580]     Content-Type: application/json
I0515 16:37:22.172626   66477 round_trippers.go:580]     Strict-Transport-Security: max-age=31536000; includeSubDomains; preload
I0515 16:37:22.172706   66477 request.go:1073] Response Body: {"kind":"Status","apiVersion":"v1","metadata":{},"status":"Failure","message":"configmaps \"motd\" is forbidden: User \"system:anonymous\" cannot get resource \"configmaps\" in API group \"\" in the namespace \"openshift\"","reason":"Forbidden","details":{"name":"motd","kind":"configmaps"},"code":403}
I0515 16:37:22.173975   66477 helpers.go:222] server response object: [{
  "metadata": {},
  "status": "Failure",
  "message": "Internal error occurred: unexpected response: 400",
  "reason": "InternalError",
  "details": {
    "causes": [
      {
        "message": "unexpected response: 400"
      }
    ]
  },
  "code": 500
}]
Error from server (InternalError): Internal error occurred: unexpected response: 400
```

## Adding the openshift-challenging-client at the OIDC side
* oc login is still broken
  * the redirect URI is wrong -> the client adds `/oauth/token/implicit` to the `redirect_uri` path but this is in fact OpenShift-specific and would not exist in the provider
  * right now we only get 400 only because of the invalid `redirect_uri`

```
...
I0515 16:41:28.523153   66921 round_trippers.go:466] curl -v -XGET  -H "X-Csrf-Token: 1" 'https://test-route-e2e-test-authentication-operator-gdw2f.apps.sl-bd.group-b.devcluster.openshift.com/realms/master/protocol/openid-connect/auth?client_id=openshift-challenging-client&code_challenge=pHansE692pMAM7zBqglQLhh9Pq1SedNQouGtUfppszY&code_challenge_method=S256&redirect_uri=https%3A%2F%2Ftest-route-e2e-test-authentication-operator-gdw2f.apps.sl-bd.group-b.devcluster.openshift.com%2Frealms%2Fmaster%2Foauth%2Ftoken%2Fimplicit&response_type=code'
I0515 16:41:28.523495   66921 round_trippers.go:495] HTTP Trace: DNS Lookup for test-route-e2e-test-authentication-operator-gdw2f.apps.sl-bd.group-b.devcluster.openshift.com resolved to [{107.22.212.24 } {54.166.125.229 }]
I0515 16:41:28.622073   66921 round_trippers.go:510] HTTP Trace: Dial to tcp:107.22.212.24:443 succeed
I0515 16:41:28.848938   66921 round_trippers.go:553] GET https://test-route-e2e-test-authentication-operator-gdw2f.apps.sl-bd.group-b.devcluster.openshift.com/realms/master/protocol/openid-connect/auth?client_id=openshift-challenging-client&code_challenge=pHansE692pMAM7zBqglQLhh9Pq1SedNQouGtUfppszY&code_challenge_method=S256&redirect_uri=https%3A%2F%2Ftest-route-e2e-test-authentication-operator-gdw2f.apps.sl-bd.group-b.devcluster.openshift.com%2Frealms%2Fmaster%2Foauth%2Ftoken%2Fimplicit&response_type=code 400 Bad Request in 325 milliseconds
I0515 16:41:28.849040   66921 round_trippers.go:570] HTTP Statistics: DNSLookup 0 ms Dial 98 ms TLSHandshake 101 ms ServerProcessing 124 ms Duration 325 ms
I0515 16:41:28.849075   66921 round_trippers.go:577] Response Headers:
I0515 16:41:28.849105   66921 round_trippers.go:580]     Content-Type: text/html;charset=utf-8
I0515 16:41:28.849132   66921 round_trippers.go:580]     Content-Length: 1776
I0515 16:41:28.849160   66921 round_trippers.go:580]     Set-Cookie: 3c9e6724823ac7ad9a5d50942d705e4c=39f2e9b0235c633edc00cae002808130; path=/; HttpOnly; Secure; SameSite=None
I0515 16:41:28.849190   66921 round_trippers.go:580]     X-Robots-Tag: none
I0515 16:41:28.849218   66921 round_trippers.go:580]     Referrer-Policy: no-referrer
I0515 16:41:28.849245   66921 round_trippers.go:580]     Content-Security-Policy: frame-src 'self'; frame-ancestors 'self'; object-src 'none';
I0515 16:41:28.849273   66921 round_trippers.go:580]     X-Xss-Protection: 1; mode=block
I0515 16:41:28.849300   66921 round_trippers.go:580]     X-Frame-Options: SAMEORIGIN
I0515 16:41:28.849326   66921 round_trippers.go:580]     X-Content-Type-Options: nosniff
I0515 16:41:28.849353   66921 round_trippers.go:580]     Strict-Transport-Security: max-age=31536000; includeSubDomains
I0515 16:41:28.849380   66921 round_trippers.go:580]     Content-Language: en
I0515 16:41:28.850560   66921 round_trippers.go:466] curl -v -XGET  -H "Accept: application/json, */*" -H "User-Agent: oc/v4.2.0 (linux/amd64) kubernetes/6e9a161" 'https://api.sl-bd.group-b.devcluster.openshift.com:6443/api/v1/namespaces/openshift/configmaps/motd'
I0515 16:41:28.949039   66921 round_trippers.go:553] GET https://api.sl-bd.group-b.devcluster.openshift.com:6443/api/v1/namespaces/openshift/configmaps/motd 403 Forbidden in 98 milliseconds
I0515 16:41:28.949099   66921 round_trippers.go:570] HTTP Statistics: GetConnection 0 ms ServerProcessing 98 ms Duration 98 ms
I0515 16:41:28.949123   66921 round_trippers.go:577] Response Headers:
I0515 16:41:28.949149   66921 round_trippers.go:580]     Audit-Id: 0367ba4d-e966-4a8f-adab-f407312a261f
I0515 16:41:28.949179   66921 round_trippers.go:580]     X-Kubernetes-Pf-Flowschema-Uid: 8db4d3d4-4c84-4a0c-9fc3-0f7e736b0bf4
I0515 16:41:28.949203   66921 round_trippers.go:580]     X-Kubernetes-Pf-Prioritylevel-Uid: e20e4cbc-8ab1-4280-8053-cdbd773f31b8
I0515 16:41:28.949226   66921 round_trippers.go:580]     Date: Mon, 15 May 2023 14:41:28 GMT
I0515 16:41:28.949251   66921 round_trippers.go:580]     Content-Length: 303
I0515 16:41:28.949275   66921 round_trippers.go:580]     Cache-Control: no-cache, private
I0515 16:41:28.949299   66921 round_trippers.go:580]     Content-Type: application/json
I0515 16:41:28.949322   66921 round_trippers.go:580]     Strict-Transport-Security: max-age=31536000; includeSubDomains; preload
I0515 16:41:28.949346   66921 round_trippers.go:580]     X-Content-Type-Options: nosniff
I0515 16:41:28.949409   66921 request.go:1073] Response Body: {"kind":"Status","apiVersion":"v1","metadata":{},"status":"Failure","message":"configmaps \"motd\" is forbidden: User \"system:anonymous\" cannot get resource \"configmaps\" in API group \"\" in the namespace \"openshift\"","reason":"Forbidden","details":{"name":"motd","kind":"configmaps"},"code":403}
I0515 16:41:28.950280   66921 helpers.go:222] server response object: [{
  "metadata": {},
  "status": "Failure",
  "message": "Internal error occurred: unexpected response: 400",
  "reason": "InternalError",
  "details": {
    "causes": [
      {
        "message": "unexpected response: 400"
      }
    ]
  },
  "code": 500
}]
Error from server (InternalError): Internal error occurred: unexpected response: 400
```

## Relaxing the validation on the redirect_uri
- login is again broken
  - I relaxed the `redirect_uri` path check. This is **NOT RECOMMENDED** by OAuth2 best practices, these require strict URI comparisons
  - The `oc` sets wrong expectations (OCP specific) on how to retrieve the token -> OIDC provider will **generally NOT send HTTP challenges** along their login form page
  - We need the `oc login` with web browser here
    - **(? unclear ?)** hopefully the providers will support the looser port validation of localhost URIs

```
...
I0515 16:48:13.181826   67968 round_trippers.go:466] curl -v -XGET  -H "X-Csrf-Token: 1" 'https://test-route-e2e-test-authentication-operator-gdw2f.apps.sl-bd.group-b.devcluster.openshift.com/realms/master/protocol/openid-connect/auth?client_id=openshift-challenging-client&code_challenge=8eztIQeYoBy6ONOWFmTAa80i9Q918R70VOUTrEVxmy8&code_challenge_method=S256&redirect_uri=https%3A%2F%2Ftest-route-e2e-test-authentication-operator-gdw2f.apps.sl-bd.group-b.devcluster.openshift.com%2Frealms%2Fmaster%2Foauth%2Ftoken%2Fimplicit&response_type=code'
I0515 16:48:13.182091   67968 round_trippers.go:495] HTTP Trace: DNS Lookup for test-route-e2e-test-authentication-operator-gdw2f.apps.sl-bd.group-b.devcluster.openshift.com resolved to [{54.166.125.229 } {107.22.212.24 }]
I0515 16:48:13.280097   67968 round_trippers.go:510] HTTP Trace: Dial to tcp:54.166.125.229:443 succeed
I0515 16:48:13.503687   67968 round_trippers.go:553] GET https://test-route-e2e-test-authentication-operator-gdw2f.apps.sl-bd.group-b.devcluster.openshift.com/realms/master/protocol/openid-connect/auth?client_id=openshift-challenging-client&code_challenge=8eztIQeYoBy6ONOWFmTAa80i9Q918R70VOUTrEVxmy8&code_challenge_method=S256&redirect_uri=https%3A%2F%2Ftest-route-e2e-test-authentication-operator-gdw2f.apps.sl-bd.group-b.devcluster.openshift.com%2Frealms%2Fmaster%2Foauth%2Ftoken%2Fimplicit&response_type=code 200 OK in 321 milliseconds
```
The 200 above is a 200 of the login form page, notice no `WWW-Authenticate` challenge in the headers below.

```
I0515 16:48:13.503800   67968 round_trippers.go:570] HTTP Statistics: DNSLookup 0 ms Dial 97 ms TLSHandshake 99 ms ServerProcessing 123 ms Duration 321 ms
I0515 16:48:13.503850   67968 round_trippers.go:577] Response Headers:
I0515 16:48:13.503894   67968 round_trippers.go:580]     Referrer-Policy: no-referrer
I0515 16:48:13.503933   67968 round_trippers.go:580]     Strict-Transport-Security: max-age=31536000; includeSubDomains
I0515 16:48:13.503972   67968 round_trippers.go:580]     X-Robots-Tag: none
I0515 16:48:13.504010   67968 round_trippers.go:580]     X-Xss-Protection: 1; mode=block
I0515 16:48:13.504046   67968 round_trippers.go:580]     X-Content-Type-Options: nosniff
I0515 16:48:13.504084   67968 round_trippers.go:580]     X-Frame-Options: SAMEORIGIN
I0515 16:48:13.504121   67968 round_trippers.go:580]     Set-Cookie: AUTH_SESSION_ID=6248afca-cc1e-46cc-8eca-fc540d1c6e4d; Version=1; Path=/realms/master/; SameSite=None; Secure; HttpOnly
I0515 16:48:13.504160   67968 round_trippers.go:580]     Set-Cookie: AUTH_SESSION_ID_LEGACY=6248afca-cc1e-46cc-8eca-fc540d1c6e4d; Version=1; Path=/realms/master/; Secure; HttpOnly
I0515 16:48:13.504198   67968 round_trippers.go:580]     Set-Cookie: KC_RESTART=eyJhbGciOiJIUzI1NiIsInR5cCIgOiAiSldUIiwia2lkIiA6ICI3YmI2YmZkYy05OTlhLTQ2NGEtODUxMC1lNmQxNWVjZjNmZTEifQ.eyJjaWQiOiJvcGVuc2hpZnQtY2hhbGxlbmdpbmctY2xpZW50IiwicHR5Ijoib3BlbmlkLWNvbm5lY3QiLCJydXJpIjoiaHR0cHM6Ly90ZXN0LXJvdXRlLWUyZS10ZXN0LWF1dGhlbnRpY2F0aW9uLW9wZXJhdG9yLWdkdzJmLmFwcHMuc2wtYmQuZ3JvdXAtYi5kZXZjbHVzdGVyLm9wZW5zaGlmdC5jb20vcmVhbG1zL21hc3Rlci9vYXV0aC90b2tlbi9pbXBsaWNpdCIsImFjdCI6IkFVVEhFTlRJQ0FURSIsIm5vdGVzIjp7ImlzcyI6Imh0dHBzOi8vdGVzdC1yb3V0ZS1lMmUtdGVzdC1hdXRoZW50aWNhdGlvbi1vcGVyYXRvci1nZHcyZi5hcHBzLnNsLWJkLmdyb3VwLWIuZGV2Y2x1c3Rlci5vcGVuc2hpZnQuY29tL3JlYWxtcy9tYXN0ZXIiLCJyZXNwb25zZV90eXBlIjoiY29kZSIsInJlZGlyZWN0X3VyaSI6Imh0dHBzOi8vdGVzdC1yb3V0ZS1lMmUtdGVzdC1hdXRoZW50aWNhdGlvbi1vcGVyYXRvci1nZHcyZi5hcHBzLnNsLWJkLmdyb3VwLWIuZGV2Y2x1c3Rlci5vcGVuc2hpZnQuY29tL3JlYWxtcy9tYXN0ZXIvb2F1dGgvdG9rZW4vaW1wbGljaXQiLCJjb2RlX2NoYWxsZW5nZV9tZXRob2QiOiJTMjU2IiwiY29kZV9jaGFsbGVuZ2UiOiI4ZXp0SVFlWW9CeTZPTk9XRm1UQWE4MGk5UTkxOFI3MFZPVVRyRVZ4bXk4In19._O2R4oHilvHczbWdc2RbreFl4jYph8sD3YwqQhNThS4; Version=1; Path=/realms/master/; Secure; HttpOnly
I0515 16:48:13.504252   67968 round_trippers.go:580]     Set-Cookie: 3c9e6724823ac7ad9a5d50942d705e4c=39f2e9b0235c633edc00cae002808130; path=/; HttpOnly; Secure; SameSite=None
I0515 16:48:13.504290   67968 round_trippers.go:580]     Content-Language: en
I0515 16:48:13.504325   67968 round_trippers.go:580]     Cache-Control: no-store, must-revalidate, max-age=0
I0515 16:48:13.504363   67968 round_trippers.go:580]     Content-Security-Policy: frame-src 'self'; frame-ancestors 'self'; object-src 'none';
I0515 16:48:13.504411   67968 round_trippers.go:580]     Content-Length: 3546
I0515 16:48:13.504451   67968 round_trippers.go:580]     Content-Type: text/html;charset=utf-8
I0515 16:48:13.505569   67968 round_trippers.go:466] curl -v -XGET  -H "Accept: application/json, */*" -H "User-Agent: oc/v4.2.0 (linux/amd64) kubernetes/6e9a161" 'https://api.sl-bd.group-b.devcluster.openshift.com:6443/api/v1/namespaces/openshift/configmaps/motd'
I0515 16:48:13.604870   67968 round_trippers.go:553] GET https://api.sl-bd.group-b.devcluster.openshift.com:6443/api/v1/namespaces/openshift/configmaps/motd 403 Forbidden in 99 milliseconds
I0515 16:48:13.604943   67968 round_trippers.go:570] HTTP Statistics: GetConnection 0 ms ServerProcessing 98 ms Duration 99 ms
I0515 16:48:13.604970   67968 round_trippers.go:577] Response Headers:
I0515 16:48:13.605002   67968 round_trippers.go:580]     Audit-Id: 93a8181d-bf45-400b-900a-2704c811f71f
I0515 16:48:13.605031   67968 round_trippers.go:580]     Cache-Control: no-cache, private
I0515 16:48:13.605059   67968 round_trippers.go:580]     X-Kubernetes-Pf-Flowschema-Uid: 8db4d3d4-4c84-4a0c-9fc3-0f7e736b0bf4
I0515 16:48:13.605087   67968 round_trippers.go:580]     Content-Length: 303
I0515 16:48:13.605113   67968 round_trippers.go:580]     Date: Mon, 15 May 2023 14:48:13 GMT
I0515 16:48:13.605141   67968 round_trippers.go:580]     Content-Type: application/json
I0515 16:48:13.605167   67968 round_trippers.go:580]     Strict-Transport-Security: max-age=31536000; includeSubDomains; preload
I0515 16:48:13.605194   67968 round_trippers.go:580]     X-Content-Type-Options: nosniff
I0515 16:48:13.605221   67968 round_trippers.go:580]     X-Kubernetes-Pf-Prioritylevel-Uid: e20e4cbc-8ab1-4280-8053-cdbd773f31b8
I0515 16:48:13.605280   67968 request.go:1073] Response Body: {"kind":"Status","apiVersion":"v1","metadata":{},"status":"Failure","message":"configmaps \"motd\" is forbidden: User \"system:anonymous\" cannot get resource \"configmaps\" in API group \"\" in the namespace \"openshift\"","reason":"Forbidden","details":{"name":"motd","kind":"configmaps"},"code":403}
I0515 16:48:13.606084   67968 helpers.go:222] server response object: [{
  "metadata": {},
  "status": "Failure",
  "message": "Internal error occurred: unexpected response: 200",
  "reason": "InternalError",
  "details": {
    "causes": [
      {
        "message": "unexpected response: 200"
      }
    ]
  },
  "code": 500
}]
Error from server (InternalError): Internal error occurred: unexpected response: 200
```
Notice the `200` from above.

### Summary
The `id_token` retrieved from the OIDC can only be used directly with `--token`:
```sh
$ oc get pods --token=$t
Error from server (Forbidden): pods is forbidden: User "https://test-route-e2e-test-authentication-operator-gdw2f.apps.sl-bd.group-b.devcluster.openshift.com/realms/master#7ebc4d10-e992-410c-98d3-d66095f9de49" cannot list resource "pods" in API group "" in the namespace "default"

```
The username observed above could further be improved with various kube-apiserver OIDC flags, possibly along with some fine-tuning of the OIDC-local client settings.

# Web interface - openshift-console

## Initial config

- console is broken 
  - the "console" client does not exist
  - console redirects to `https://<OIDC_AUTHZ_ENDPOINT_URL>?client_id=console&redirect_uri=https%3A%2F%2F<console_url>%2Fauth%2Fcallback&response_type=code&scope=user%3Afull&state=c0ff9e95`

## Create the client in the OIDC

- Keycloak does not allow setting your own client secret (this might differ with other OIDC providers)
- the console-operator stomps on changes to the console OAuthClient


1. create a "console" client on the OIDC side
2. unmanage the `console-operator` deployment and scale replicas to 0
3. edit the `openshift-console/console-oauth-config` with the client secret from the OIDC
   - `console-oauth-config` only contains the `"clientSecret"` key -> the client ID is hardcoded (although the internal API/flags of the console allow setting it)
   - don't forget to base64-encode the secret from the OIDC again if modifying the secret directly in the secret
4. delete all console pods just to make sure they pick up the new secret

**Console gets stuck in a redirect** loop (old bug I reported in OCP 4.5: https://issues.redhat.com/browse/OCPBUGS-8777). No useful logs but looking into Keycloak logs we can see that the scope `user:full` is unknown.

## Add the custom scope

- Keycloak allows creating custom scopes and adding them to the clients

**Result:**

New redirect loop, there are authorization errors.

_Assumption:_ the user is missing bindings to the `system:authenticated:oauth` group.

- Adding the `system:authenticated` group to `self-provisioners` clusterrolebinding does not seem to help.
- Adding `system:authenticated` in the `cluster-admin` clusterrolebinding does not help.

Console repeatedly logs:
```
I0516 10:35:40.138269       1 metrics.go:99] auth.Metrics loginSuccessfulSync - increase metric for role "unknown"
I0516 10:35:41.426775       1 proxy.go:115] PROXY: `https://kubernetes.default.svc/apis/project.openshift.io/v1/projectrequests`
I0516 10:35:41.426859       1 proxy.go:115] PROXY: `https://kubernetes.default.svc/apis/apps.openshift.io/v1`
I0516 10:35:41.427180       1 proxy.go:115] PROXY: `https://kubernetes.default.svc/apis/config.openshift.io/v1/clusterversions/version`
I0516 10:35:41.427295       1 proxy.go:115] PROXY: `https://kubernetes.default.svc/apis/authorization.k8s.io/v1/selfsubjectaccessreviews`
I0516 10:35:41.427369       1 proxy.go:115] PROXY: `https://kubernetes.default.svc/apis/user.openshift.io/v1/users/~`
I0516 10:35:41.427445       1 proxy.go:115] PROXY: `https://kubernetes.default.svc/apis/authorization.k8s.io/v1/selfsubjectaccessreviews`
I0516 10:35:41.427525       1 proxy.go:115] PROXY: `https://kubernetes.default.svc/apis/authorization.k8s.io/v1/selfsubjectaccessreviews`
I0516 10:35:41.427583       1 proxy.go:115] PROXY: `https://kubernetes.default.svc/apis/authorization.k8s.io/v1/selfsubjectaccessreviews`
I0516 10:35:41.427841       1 proxy.go:115] PROXY: `https://kubernetes.default.svc/apis/authorization.k8s.io/v1/selfsubjectaccessreviews`
I0516 10:35:41.428337       1 proxy.go:115] PROXY: `https://kubernetes.default.svc/apis/authorization.k8s.io/v1/selfsubjectaccessreviews`
I0516 10:35:41.446268       1 proxy.go:115] PROXY: `https://kubernetes.default.svc/apis/authorization.k8s.io/v1/selfsubjectaccessreviews`
I0516 10:35:41.446355       1 proxy.go:115] PROXY: `https://kubernetes.default.svc/openapi/v2`
I0516 10:35:41.463371       1 proxy.go:115] PROXY: `https://kubernetes.default.svc/openapi/v2`
I0516 10:35:41.525070       1 proxy.go:115] PROXY: `https://kubernetes.default.svc/apis/metal3.io/v1alpha1/provisionings/provisioning-configuration`
I0516 10:35:41.525124       1 proxy.go:115] PROXY: `https://kubernetes.default.svc/apis/authorization.k8s.io/v1/selfsubjectaccessreviews`
I0516 10:35:41.525133       1 proxy.go:115] PROXY: `https://kubernetes.default.svc/apis/authorization.k8s.io/v1/selfsubjectaccessreviews`
I0516 10:35:41.525178       1 proxy.go:115] PROXY: `https://kubernetes.default.svc/apis/authorization.k8s.io/v1/selfsubjectaccessreviews`
I0516 10:35:41.525335       1 proxy.go:115] PROXY: `https://kubernetes.default.svc/apis/config.openshift.io/v1/infrastructures/cluster`
I0516 10:35:41.525081       1 proxy.go:115] PROXY: `https://kubernetes.default.svc/apis/authorization.k8s.io/v1/selfsubjectaccessreviews`
I0516 10:35:41.695630       1 metrics.go:61] auth.Metrics LoginRequested
I0516 10:35:41.954456       1 auth.go:418] oauth success, redirecting to: "https://console-openshift-console.apps.sl-bd.group-b.devcluster.openshift.com/"
E0516 10:35:41.963528       1 metrics.go:156] Error in auth.metrics isKubeAdmin: Unauthorized
E0516 10:35:41.970919       1 metrics.go:142] Error in auth.metrics canGetNamespaces: Unauthorized
```

Apparently the console has trouble acting on behalf of the user, the last log line would come from self-SAR.

In the audit logs we can see:
```
{"kind":"Event","apiVersion":"audit.k8s.io/v1","level":"Metadata","auditID":"c771f01d-5995-40e4-9259-879773112159","stage":"ResponseStarted","requestURI":"/apis/authorization.k8s.io/v1/selfsubjectaccessreviews","verb":"create","user":{},"sourceIPs":["10.0.152.130"],"objectRef":{"resource":"selfsubjectaccessreviews","apiGroup":"authorization.k8s.io","apiVersion":"v1"},"responseStatus":{"metadata":{},"status":"Failure","message":"Unauthorized","reason":"Unauthorized","code":401},"requestReceivedTimestamp":"2023-05-16T10:27:28.826058Z","stageTimestamp":"2023-05-16T10:27:28.837875Z"}
```

Note the `"user":{}`.

## Configuring the OIDC options for console directly (== don't use OpenShift auth?)

**Result**: another redirect loop.
```
W0516 11:13:51.377143       1 main.go:226] Flag inactivity-timeout is set to less then 300 seconds and will be ignored!
I0516 11:13:51.377183       1 main.go:370] cookies are secure!
I0516 11:13:51.408440       1 main.go:835] Binding to [::]:8443...
I0516 11:13:51.408471       1 main.go:837] using TLS
I0516 11:13:54.408793       1 metrics.go:141] serverconfig.Metrics: Update ConsolePlugin metrics...
I0516 11:13:54.417286       1 metrics.go:151] serverconfig.Metrics: Update ConsolePlugin metrics: &map[] (took 8.473194ms)
I0516 11:13:56.408953       1 metrics.go:88] usage.Metrics: Count console users...
I0516 11:13:56.516563       1 metrics.go:117] usage.Metrics: Ignore role binding: "console-user-settings-admin" (name doesn't match user-settings-*-rolebinding)
I0516 11:13:56.616716       1 metrics.go:117] usage.Metrics: Ignore role binding: "system:deployers" (name doesn't match user-settings-*-rolebinding)
I0516 11:13:56.716912       1 metrics.go:117] usage.Metrics: Ignore role binding: "system:image-builders" (name doesn't match user-settings-*-rolebinding)
I0516 11:13:56.817148       1 metrics.go:117] usage.Metrics: Ignore role binding: "system:image-pullers" (name doesn't match user-settings-*-rolebinding)
I0516 11:13:56.817185       1 metrics.go:174] usage.Metrics: Update console users metrics: 0 kubeadmin, 0 cluster-admins, 0 developers, 0 unknown/errors (took 408.205717ms)
I0516 11:14:06.148484       1 middleware.go:40] authentication failed: No session found on server
I0516 11:14:20.653464       1 middleware.go:40] authentication failed: No session found on server
I0516 11:14:36.144097       1 middleware.go:40] authentication failed: No session found on server
I0516 11:14:50.649991       1 middleware.go:40] authentication failed: No session found on server
I0516 11:15:06.143964       1 middleware.go:40] authentication failed: No session found on server
I0516 11:15:20.650244       1 middleware.go:40] authentication failed: No session found on server
I0516 11:15:36.144338       1 middleware.go:40] authentication failed: No session found on server
I0516 11:15:50.650321       1 middleware.go:40] authentication failed: No session found on server
I0516 11:16:06.144880       1 middleware.go:40] authentication failed: No session found on server
I0516 11:16:20.649929       1 middleware.go:40] authentication failed: No session found on server
I0516 11:16:36.144772       1 middleware.go:40] authentication failed: No session found on server
I0516 11:16:50.649536       1 middleware.go:40] authentication failed: No session found on server
I0516 11:17:00.732471       1 middleware.go:40] authentication failed: http: named cookie not present
I0516 11:17:01.405478       1 middleware.go:40] authentication failed: http: named cookie not present
I0516 11:17:02.248920       1 middleware.go:40] authentication failed: http: named cookie not present
I0516 11:17:03.362388       1 middleware.go:40] authentication failed: http: named cookie not present
I0516 11:17:04.862731       1 middleware.go:40] authentication failed: http: named cookie not present
I0516 11:17:05.029879       1 middleware.go:40] authentication failed: http: named cookie not present
I0516 11:17:05.030146       1 middleware.go:40] authentication failed: http: named cookie not present
I0516 11:17:05.030156       1 middleware.go:40] authentication failed: http: named cookie not present
I0516 11:17:05.034345       1 middleware.go:40] authentication failed: http: named cookie not present
I0516 11:17:05.150850       1 middleware.go:40] authentication failed: http: named cookie not present
I0516 11:17:05.277770       1 metrics.go:61] auth.Metrics LoginRequested
E0516 11:17:05.761578       1 auth.go:388] missing auth code in query param
I0516 11:17:05.761593       1 metrics.go:107] auth.Metrics LoginFailed with reason "unknown"
I0516 11:17:06.144158       1 middleware.go:40] authentication failed: No session found on server
I0516 11:17:06.935895       1 middleware.go:40] authentication failed: http: named cookie not present
...
```

On the Keycloak side though:
```
023-05-16 11:18:44,711 WARN  [org.keycloak.events] (executor-thread-38) type=LOGIN_ERROR, realmId=0380c1d5-86ab-4606-abec-d33b6652c352, clientId=console, userId=null, ipAddress=213.175.37.10, error=invalid_request, response_type=code, redirect_uri=https://console-openshift-console.apps.sl-bd.group-b.devcluster.openshift.com/auth/callback, response_mode=query
```

==> We may want to configure additional scopes for the client, it's using https://github.com/openshift/console/blob/3e0bb0928ce09030bc3340c9639b2a1df9e0a007/cmd/bridge/main.go#L614

* `"groups"` is not a standard scope! [OIDC Core Spec - Requesting Claims using Scope Values](https://openid.net/specs/openid-connect-core-1_0.html#ScopeClaims)

## Add 'groups' among client scopes in Keycloak

==> another redirect loop

console logs:
```
W0516 11:27:28.351058       1 main.go:226] Flag inactivity-timeout is set to less then 300 seconds and will be ignored!
I0516 11:27:28.351098       1 main.go:370] cookies are secure!
I0516 11:27:28.457933       1 main.go:835] Binding to [::]:8443...
I0516 11:27:28.458050       1 main.go:837] using TLS
I0516 11:27:31.462739       1 metrics.go:141] serverconfig.Metrics: Update ConsolePlugin metrics...
I0516 11:27:31.472028       1 metrics.go:151] serverconfig.Metrics: Update ConsolePlugin metrics: &map[] (took 9.268829ms)
I0516 11:27:33.462226       1 metrics.go:88] usage.Metrics: Count console users...
I0516 11:27:33.566037       1 metrics.go:117] usage.Metrics: Ignore role binding: "console-user-settings-admin" (name doesn't match user-settings-*-rolebinding)
I0516 11:27:33.666267       1 metrics.go:117] usage.Metrics: Ignore role binding: "system:deployers" (name doesn't match user-settings-*-rolebinding)
I0516 11:27:33.766544       1 metrics.go:117] usage.Metrics: Ignore role binding: "system:image-builders" (name doesn't match user-settings-*-rolebinding)
I0516 11:27:33.866779       1 metrics.go:117] usage.Metrics: Ignore role binding: "system:image-pullers" (name doesn't match user-settings-*-rolebinding)
I0516 11:27:33.866799       1 metrics.go:174] usage.Metrics: Update console users metrics: 0 kubeadmin, 0 cluster-admins, 0 developers, 0 unknown/errors (took 404.554644ms)
I0516 11:27:45.071282       1 middleware.go:40] authentication failed: No session found on server
I0516 11:27:47.044653       1 middleware.go:40] authentication failed: No session found on server
I0516 11:27:49.780316       1 middleware.go:40] authentication failed: http: named cookie not present
I0516 11:27:50.393077       1 middleware.go:40] authentication failed: http: named cookie not present
I0516 11:27:51.124024       1 middleware.go:40] authentication failed: http: named cookie not present
I0516 11:27:52.054570       1 middleware.go:40] authentication failed: http: named cookie not present
I0516 11:27:53.405513       1 middleware.go:40] authentication failed: http: named cookie not present
I0516 11:27:53.565205       1 middleware.go:40] authentication failed: http: named cookie not present
I0516 11:27:53.565332       1 middleware.go:40] authentication failed: http: named cookie not present
I0516 11:27:53.566918       1 middleware.go:40] authentication failed: http: named cookie not present
I0516 11:27:53.570078       1 middleware.go:40] authentication failed: http: named cookie not present
I0516 11:27:53.684323       1 middleware.go:40] authentication failed: http: named cookie not present
I0516 11:27:53.797454       1 metrics.go:61] auth.Metrics LoginRequested
I0516 11:28:02.314513       1 session.go:83] Pruned 0 expired sessions.
I0516 11:28:02.314540       1 auth.go:418] oauth success, redirecting to: "https://console-openshift-console.apps.sl-bd.group-b.devcluster.openshift.com/"
E0516 11:28:02.350480       1 metrics.go:156] Error in auth.metrics isKubeAdmin: Unauthorized
E0516 11:28:02.363748       1 metrics.go:142] Error in auth.metrics canGetNamespaces: Unauthorized
```

And again, no user in the `self-SARs` in the audit logs. The OIDC-specific config got us nowhere.

This is because the web console appears to expect the "name" claim in the id_token but OIDC makes no assurances this is present by default. Related code: https://github.com/openshift/console/blob/3e0bb0928ce09030bc3340c9639b2a1df9e0a007/pkg/auth/loginstate.go#L30-L56

# oauth-proxy

## Initial config

- oauth proxy from https://github.com/openshift/oauth-proxy/blob/master/contrib/sidecar.yaml
   - **broken** - client does not exist, the OIDC will not respect our oauth clients being constructed from SAs

## Config the client on Keycloak side

- we can set the client-id and client-secret explicitly
  - **broken** - unknown scopes

- create the scopes in Keycloak and adding them to the client
  - **broken**
    - the code is trying to retrieve user attributes:
  https://github.com/openshift/oauth-proxy/blob/1e2bb287586c22fa76ac1be84030148f8c1f95d3/providers/openshift/provider.go#L462-L487
    - it is trying to reach the /apis/user.openshift.io/v1/users/~ endpoint but fails with:
```
2023/05/16 11:57:42 provider.go:593: 401 GET https://172.30.0.1/apis/user.openshift.io/v1/users/~ {"kind":"Status","apiVersion":"v1","metadata":{},"status":"Failure","message":"Unauthorized","reason":"Unauthorized","code":401}
2023/05/16 11:57:42 oauthproxy.go:646: error redeeming code (client:10.131.0.34:50820): unable to retrieve email address for user from token: got 401 {"kind":"Status","apiVersion":"v1","metadata":{},"status":"Failure","message":"Unauthorized","reason":"Unauthorized","code":401}
2023/05/16 11:57:42 oauthproxy.go:439: ErrorPage 500 Internal Error Internal Error
```

There is only very little appetite to get oauth-proxy working in these scenarios though. Why use it when OIDC could be used directly?

I'm not going to proceed troubleshooting this further.
