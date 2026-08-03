# fb2cng-web

Web interface for [fb2cng](https://github.com/rupor-github/fb2cng): drop `fb2` / `fb2.zip`
files, get EPUB (or kepub/kfx/azw8/pdf) back automatically.

## Run

    docker build -t fb2cng-web .
    docker run --rm -p 8080:8080 fb2cng-web

> The image is built on BellSoft Alpaquita Linux (musl); the production runtime stage uses
> the hardened Alpaquita base (`bellsoft/hardened-base:musl`) — minimal, non-root (UID 65532),
> no shell or package manager. The app writes conversion temp files to `/tmp`; the hardened
> base ships a writable `/tmp`, so this works out of the box. If you run with a read-only root
> filesystem, mount a writable `/tmp` (e.g. `--tmpfs /tmp:rw,mode=1777`, as the compose example shows).

Open http://localhost:8080. Drop your books on the **Convert** page and pick a preset —
defaults work out of the box.

The **Settings** tab manages reusable **presets**: create, duplicate, delete, or set a
default. Each preset opens a full **option editor** (search, grouped sections, per-option
help, live "changed" markers) covering every `fbc` option — format, ToC, images,
footnotes, filename templates, and more. Overrides are stored sparsely (only what differs
from the fbc defaults), and an **effective-config** pane shows the merged YAML that will
actually run.

## Versioning & images

Images are published to GitHub Container Registry on every push to `main` and
whenever a new [`fbc`](https://github.com/rupor-github/fb2cng) release appears.

Tags follow `<app-version>-<fbc-version>` (the `fbc` tag's leading `v` is dropped),
e.g. `1-1.4.5`, plus a moving `latest`:

    docker pull ghcr.io/<owner>/fb2cng-web:latest
    docker pull ghcr.io/<owner>/fb2cng-web:1-1.4.5

- **`VERSION`** holds the integer app version. Bump it by hand when the app code changes.
- **`FBC_VERSION`** holds the pinned `fbc` release. A daily GitHub Actions job
  (`fbc-update`) checks upstream; on a new release it rewrites `FBC_VERSION`,
  commits the bump, and publishes a fresh `<VERSION>-<new-fbc>` image (and `latest`).

Images are multi-arch (`linux/amd64`, `linux/arm64`); Docker pulls the right one
automatically.

> First-time setup: the GHCR package is created on the first successful push and
> defaults to **private**. Make it public (or grant pull access) in the repo's
> Packages settings if anonymous pulls are wanted.

### Local development

Version bumps are automated with [lefthook](https://github.com/evilmartians/lefthook).
After cloning, run once:

    lefthook install

Then any commit that touches app or build code (`*.go`, `go.mod`/`go.sum`,
`Dockerfile`, `internal/web/*`) auto-increments `VERSION`. Doc-, CI-, and
`FBC_VERSION`-only commits leave it untouched. The hook is local-only — it does
not run in CI, so install it after cloning.

## Configuration (env)

| Var | Default | Meaning |
|-----|---------|---------|
| `PORT` | `8080` | listen port |
| `FBC_BIN` | `fbc` | path to the fbc binary |
| `MAX_CONCURRENT` | `3` | max simultaneous conversions |
| `PRESETS_DIR` | per-user config dir | where presets are stored (persistent) |
| `JOBS_DIR` | per-user cache dir | scratch dir for in-flight conversions (swept on TTL) |
| `JOBS_TTL` | `1h` | max age of a finished job before its files are swept |
| `FBC_TIMEOUT` | `10m` | max time a single `fbc` conversion may run before it's killed |

> In the Docker image, `FBC_BIN` is preset to `/usr/local/bin/fbc`, `PRESETS_DIR` to
> `/data/presets` (mount a volume there to persist presets), and `JOBS_DIR` to
> `/tmp/fb2cng-jobs`. Outside the container, unset `PRESETS_DIR`/`JOBS_DIR` default to your
> OS per-user config/cache dirs (e.g. `~/.config/fb2cng/presets`, `~/.cache/fb2cng/jobs`).

## Authentication (optional)

Set `AUTH_MODE=oidc` to require OpenID Connect login; the default `off` leaves the
app open. In `oidc` mode the app runs the authorization-code flow itself and gates
access on a group claim.

| Env var | Meaning | Default |
| --- | --- | --- |
| `AUTH_MODE` | `off` or `oidc` | `off` |
| `AUTH_OIDC_ISSUER` | issuer / discovery base URL | — |
| `AUTH_OIDC_CLIENT_ID` | OIDC client id | — |
| `AUTH_OIDC_CLIENT_SECRET` | OIDC client secret (plaintext) | — |
| `AUTH_OIDC_REDIRECT_URL` | absolute callback URL (`https://host/auth/callback`) | — |
| `AUTH_OIDC_GROUPS_CLAIM` | ID-token claim holding group membership | `groups` |
| `AUTH_OIDC_REQUIRED_GROUP` | group value required for access | — |
| `AUTH_SESSION_KEY` | base64 HMAC key (32+ bytes); random if unset | random (logs a warning) |
| `AUTH_SESSION_TTL` | session lifetime / revalidation interval | `8h` |

The group is read from the **ID token**; some providers need explicit config to put
it there. See [docs/oidc-authelia.md](docs/oidc-authelia.md) for an Authelia walkthrough.
