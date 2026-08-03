# Using Authelia as the OIDC provider

fb2cng-web authenticates as an OpenID Connect **Relying Party**. This guide sets
up [Authelia](https://www.authelia.com/) (4.39+) as the **provider (IdP)** and
gates access on group membership.

The app reads the group from the **ID token**. Authelia does not put `groups`
into the ID token by default (scopes only expose it at the UserInfo endpoint), so
a small `claims_policies` block is required — step 3 below.

## 1. Create the group and assign the user

The required group must exist and the user must be a member. With the file-based
backend (`users_database.yml`):

```yaml
users:
  alice:
    disabled: false
    displayname: 'Alice'
    password: '$argon2id$...'   # your existing hash
    email: 'alice@example.com'
    groups:
      - 'fb2cng-users'          # <- the group the app will require
```

(With the LDAP backend, add the user to the equivalent group there instead.)

## 2. Generate a client secret

Authelia stores the client secret **hashed**; the app is configured with the
**plaintext**. Generate both at once:

```bash
authelia crypto hash generate pbkdf2 \
  --variant sha512 --random --random.length 72 --random.charset rfc3986
```

Keep the `Random Password` (plaintext → app) and the `Digest` (hash → Authelia).

## 3. Register the client

In Authelia's configuration, under `identity_providers.oidc`:

```yaml
identity_providers:
  oidc:
    ## ...existing hmac_secret / jwks issuer keys stay as they are...

    claims_policies:
      ## Break-glass: inject `groups` into the ID token so the app can read it.
      fb2cng:
        id_token: ['groups']

    clients:
      - client_id: 'fb2cng'
        client_name: 'fb2cng-web'
        client_secret: '$pbkdf2-sha512$310000$...'   # the Digest from step 2
        public: false
        authorization_policy: 'two_factor'            # or 'one_factor'
        claims_policy: 'fb2cng'                        # <- assigns the policy above
        require_pkce: true
        pkce_challenge_method: 'S256'
        redirect_uris:
          - 'https://fb2cng.example.com/auth/callback' # must match AUTH_OIDC_REDIRECT_URL
        scopes:
          - 'openid'
          - 'profile'
          - 'email'
          - 'groups'
        response_types:
          - 'code'
        grant_types:
          - 'authorization_code'
        token_endpoint_auth_method: 'client_secret_basic'
```

Reload Authelia.

## 4. Configure the app

Set these environment variables on fb2cng-web:

```bash
AUTH_MODE=oidc
AUTH_OIDC_ISSUER=https://auth.example.com          # Authelia's base URL
AUTH_OIDC_CLIENT_ID=fb2cng
AUTH_OIDC_CLIENT_SECRET=<Random Password from step 2>   # plaintext
AUTH_OIDC_REDIRECT_URL=https://fb2cng.example.com/auth/callback
AUTH_OIDC_GROUPS_CLAIM=groups                      # default; matches the claim above
AUTH_OIDC_REQUIRED_GROUP=fb2cng-users              # must match step 1
AUTH_SESSION_KEY=<base64 32-byte key>              # persist across restarts
AUTH_SESSION_TTL=8h                                # revalidation interval (see below)
```

Generate a session key:

```bash
openssl rand -base64 32
```

The app discovers the provider at
`${AUTH_OIDC_ISSUER}/.well-known/openid-configuration` on startup; an unreachable
or wrong issuer is fatal.

## 5. Verify

1. Open the app — you should be redirected to Authelia.
2. Log in as a member of `fb2cng-users` → back to the app, authenticated.
3. Log in as a **non-member** → `403` (authenticated at the IdP, but not in the
   group).

## Revocation

There is no per-request call back to Authelia. `AUTH_SESSION_TTL` is the
revalidation interval: when a session expires the app bounces the user through
Authelia again — silently if their Authelia session is still alive — and
re-checks the group. Lower it (e.g. `30m`) for tighter revocation; the re-check
stays invisible to the user.
