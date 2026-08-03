# OIDC authentication — design

## Summary

Replace the current reverse-proxy forward-auth with the app acting as its own
OIDC Relying Party (RP). The app runs the authorization-code flow directly,
verifies the ID token, gates access on a group claim, and maintains its own
signed session cookie. Auth is optional: `off` (default) or `oidc`.

## Motivation

Today the app performs no authentication itself. With `AUTH_FORWARD_AUTH=true`
it trusts `Remote-User` / `Remote-Name` headers set by a reverse proxy
(Authelia), guarded by an optional trusted-source-IP allowlist. That works but
requires operating a separate forward-auth proxy. Built-in OIDC lets the app be
deployed behind any plain reverse proxy (or none) and authenticate directly
against an IdP.

The existing forward-auth mechanism is removed, not kept alongside — the project
carries no backwards-compatibility burden (no external API consumers).

## Scope

In scope:

- App-side OIDC authorization-code flow (with PKCE + nonce + state).
- ID-token verification via the IdP's discovery document and JWKS.
- Authorization gate on a configurable group claim.
- Own signed session cookie with an app-controlled TTL.
- Login / callback / logout routes.
- Config changes and startup validation.

Out of scope (addable later, deliberately skipped):

- Refresh tokens / per-request re-validation against the IdP. Session TTL is the
  revocation-latency dial instead (see Session below).
- RP-initiated logout (`end_session_endpoint`). Logout is local-only.
- Encrypted (vs signed) session cookie — the cookie holds `sub`, display name,
  and expiry, none secret; integrity (signing) is the requirement, not
  confidentiality.
- Per-user data isolation — presets, jobs, and status are global in this app;
  auth is a gate, not a tenancy boundary.

## Architecture

New package `internal/auth` owns everything OIDC and session related. It exposes:

- A constructor that performs OIDC discovery at startup and builds the provider,
  verifier, and `oauth2.Config`.
- HTTP handlers for `/auth/login`, `/auth/callback`, `/auth/logout`.
- Middleware that gates the application routes.
- A way for the server to read the current session (for the header badge).

Dependencies (both standard for Go OIDC, and in the "don't hand-roll crypto"
zone):

- `github.com/coreos/go-oidc/v3/oidc` — discovery, JWKS, ID-token verification.
- `golang.org/x/oauth2` — authorization-code exchange.

Session signing and cookie handling use only the standard library.

## Flow

1. Unauthenticated request to a gated route → `302` to `/auth/login` with the
   originally requested path preserved (e.g. in a short-lived cookie or the
   `state` round-trip).
2. `/auth/login` — generate `state`, `nonce`, and a PKCE verifier (S256). Store
   them in a short-lived, HttpOnly temp cookie. Redirect to the IdP
   authorization endpoint.
3. `/auth/callback?code&state` —
   - verify `state` matches the temp cookie (CSRF protection);
   - exchange `code` for tokens using the PKCE verifier;
   - verify the ID token: signature (JWKS), audience, expiry, and `nonce`;
   - check the required group claim (see Authorization);
   - on success, set the session cookie and redirect to the original path (or `/`);
   - on any failure, clear the temp cookie and return a generic error page /
     `403` without leaking specifics.
4. `/auth/logout` — clear the session cookie, redirect to `/`.

`/auth/*` and `/static/*` are always public (the login/error pages need CSS).
Every other route is gated.

## Authorization

After the IdP authenticates the user, access additionally requires a group
claim match:

- `AUTH_OIDC_GROUPS_CLAIM` — the claim to read (default `groups`).
- `AUTH_OIDC_REQUIRED_GROUP` — the value that must be present in that claim.

The claim is read from the **verified ID token** and is expected to be a JSON
array of strings; membership is "required group is one of the values". If the
claim is absent or does not contain the required group → `403`.

Because the group is read from the ID token, the IdP must place it there. Some
providers (Authelia 4.39+, others) only expose scope-derived claims at the
UserInfo endpoint by default and require an explicit policy to inject `groups`
into the ID token. Operator setup for that is documented per-provider — see
[docs/oidc-authelia.md](../../oidc-authelia.md) for the Authelia walkthrough
(`claims_policies`).

## Session

The app issues its own **signed** session cookie, independent of the (typically
short) ID-token lifetime.

- Contents: `sub`, display name (from `name`, falling back to `email`, then
  `sub`), and an absolute expiry timestamp.
- Signing: HMAC-SHA256 over the encoded payload, constant-time compared on read.
  Implemented in stdlib (~30 lines).
- Cookie attributes: `HttpOnly`, `Secure`, `SameSite=Lax`.
- `AUTH_SESSION_TTL` — session lifetime, default `8h`.
- `AUTH_SESSION_KEY` — base64-encoded HMAC key. If unset, a random key is
  generated at startup **and a warning is logged**: sessions will not survive a
  process restart (this app's image republishes ~daily, so a persistent key is
  recommended in production).

### Revocation

There is no per-request call to the IdP and no stored refresh token. Instead,
the session TTL doubles as the re-validation interval: when a session expires,
the user is bounced to `/auth/login`; if the IdP session is still alive the IdP
redirects straight back with no prompt (silent SSO), and the group claim is
re-verified on that round-trip. Lowering `AUTH_SESSION_TTL` (e.g. to `30m`)
tightens revocation latency without any extra machinery and stays invisible to
the user.

## Configuration

Removed from `config.Config` and `FromEnv`:

- `ForwardAuth` (`AUTH_FORWARD_AUTH`)
- `TrustedProxies` (`TRUSTED_PROXIES`)

Added:

| Env var | Meaning | Default |
| --- | --- | --- |
| `AUTH_MODE` | `off` or `oidc` | `off` |
| `AUTH_OIDC_ISSUER` | issuer / discovery base URL | — |
| `AUTH_OIDC_CLIENT_ID` | OIDC client id | — |
| `AUTH_OIDC_CLIENT_SECRET` | OIDC client secret | — |
| `AUTH_OIDC_REDIRECT_URL` | absolute callback URL (`https://host/auth/callback`) | — |
| `AUTH_OIDC_GROUPS_CLAIM` | claim holding group membership | `groups` |
| `AUTH_OIDC_REQUIRED_GROUP` | group value required for access | — |
| `AUTH_SESSION_KEY` | base64 HMAC key | random (with warning) |
| `AUTH_SESSION_TTL` | session lifetime | `8h` |

Startup validation: when `AUTH_MODE=oidc`, the issuer, client id, client secret,
redirect URL, and required group must be set — otherwise the app exits with a
clear message. Provider discovery runs at startup; an unreachable or invalid
issuer is fatal.

## Server wiring

- `Server.Handler()` wraps the mux with the auth middleware instead of
  `ForwardAuth`, and registers the `/auth/*` routes ahead of the gate.
- `Server.userLabel(r)` reads the display name from the session (empty when
  `AUTH_MODE=off`) instead of from `Remote-*` headers.
- `main.go` startup log line drops `ForwardAuth` in favor of `AUTH_MODE`.

## Error handling

- Callback failures (state mismatch, exchange error, token verification failure,
  group check failure) render a generic error page or `403`; details go to the
  server log, not the response.
- Missing/expired/tampered session cookie → treated as unauthenticated → redirect
  to login.
- Startup misconfiguration (missing required env, unreachable issuer) → fatal
  with an actionable message.

## Testing

- Session cookie: sign/verify roundtrip; tampered payload rejected; expired
  cookie rejected.
- Group-claim check: allowed and denied cases; claim absent; claim not an array.
- Middleware: no cookie → redirect; valid session → pass through; expired → redirect;
  `/auth/*` and `/static/*` reachable while unauthenticated.
- Config: `AUTH_MODE=oidc` with a missing required field fails validation.
- Full callback/exchange against a **fake IdP**: an `httptest` server serving a
  discovery document, a JWKS, and a signed ID token; drive `/auth/callback` and
  assert a session cookie is set on success and rejected on a bad group / bad
  signature / bad nonce.

## Migration / docs

- Update the README auth section: remove forward-auth instructions, document the
  OIDC env vars, and link the Authelia guide.
- `docs/oidc-authelia.md` — Authelia-as-provider walkthrough (group, client
  registration, `claims_policies` for the ID-token group claim, app env vars).
- Update `docker-compose.example.yml` to show the OIDC env vars.
