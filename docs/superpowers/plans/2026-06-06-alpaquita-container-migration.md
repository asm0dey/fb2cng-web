# Alpaquita Container Migration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Migrate every Dockerfile stage of `fb2cng-web` to BellSoft Alpaquita Linux (musl), with the production runtime stage on the **hardened** Alpaquita base.

**Architecture:** The Dockerfile has three stages — (1) fetch the `fbc` binary, (2) build the Go server, (3) runtime. We rebase each: stage 1 → `bellsoft/alpaquita-linux-base:stream-musl` (apk available for curl/unzip), stage 2 → `bellsoft/alpaquita-linux-go:1.26.3-musl`, stage 3 → `bellsoft/hardened-base:musl` (minimal, distroless-style, non-root). The Go binary is built `CGO_ENABLED=0` (static), so it runs on any libc; the downloaded `fbc` binary is verified static via a real in-container conversion smoke test. The CA-cert bundle is copied explicitly from the fetch stage so behavior is preserved even though the hardened base has no package manager.

**Tech Stack:** Docker multi-stage build, Go 1.26, BellSoft Alpaquita Linux (musl + hardened-base), `apk`.

---

## Scope

In scope: the three stages of `./Dockerfile` (the only containers we build).

Out of scope (confirmed with user): the upstream `authelia/authelia:4` and `caddy:2` images in `docker-compose.example.yml`. They are third-party published images and cannot be rebased onto Alpaquita without forking each project's build. They stay as-is.

## Key facts established during research

- Alpaquita is Alpine-compatible and uses `apk`. The musl variant is 100% stock-musl compatible.
- Build image: `bellsoft/alpaquita-linux-go:1.26.3-musl` (latest published Alpaquita Go musl tag; no `1.26.4-musl` exists yet). Because it ships Go 1.26.3 with `GOTOOLCHAIN=local`, the `go.mod` directive is lowered `1.26.4` → `1.26.3` to keep the build hermetic (local dev on 1.26.4 still satisfies the 1.26.3 minimum).
- Hardened images: `bellsoft/hardened-base:musl` (runtime) and `bellsoft/hardened-go:1.25-*` (build — NOT used here; we keep the regular `alpaquita-linux-go` builder because the build stage is discarded and we need Go 1.26).
- The hardened base is distroless-style: **no shell, no package manager**. Therefore the runtime stage must not run `apk`/`apt`; everything it needs is `COPY`-ed in.
- Distroless/hardened convention: default non-root user **UID 65532**. The original Dockerfile used `USER nobody`; we switch to numeric `USER 65532:65532` because a hardened image may lack an `/etc/passwd` `nobody` entry.
- The server (`internal/`) makes **no outbound HTTPS** itself — the only `https://` is a browser-side CSS CDN link in `index.html`. CA certs are therefore not strictly required by the app, but we copy the bundle anyway to preserve original behavior with zero risk.
- `internal/convert/runner.go` uses `os.MkdirTemp("")`, i.e. it writes to `$TMPDIR` / `/tmp`. The runtime must have a writable `/tmp`. We set `ENV TMPDIR=/tmp` and verify writability with a real conversion in the smoke test; compose documents a `tmpfs` mount.
- Routes (`internal/server/server.go`): `GET /defaults` (runs `fbc dumpconfig` → exercises the fbc binary), `POST /convert` (runs a real conversion → exercises fbc + `/tmp`), `GET /` (static files). These are the smoke-test targets.
- `.dockerignore` excludes `testdata`, so `testdata/sample.fb2` is NOT in the image — the smoke test posts it from the host over HTTP.

## Target final Dockerfile (reference — built up incrementally by the tasks below)

```dockerfile
# syntax=docker/dockerfile:1

# --- Stage 1: fetch the fbc binary ---
FROM bellsoft/alpaquita-linux-base:stream-musl AS fbc
ARG FBC_VERSION=v1.4.5
ARG FBC_ASSET=fbc-linux-amd64.zip
RUN apk add --no-cache curl unzip ca-certificates \
 && curl -fsSL -o /tmp/fbc.zip \
      "https://github.com/rupor-github/fb2cng/releases/download/${FBC_VERSION}/${FBC_ASSET}" \
 && unzip -o /tmp/fbc.zip -d /opt \
 && chmod +x /opt/fbc

# --- Stage 2: build the Go server ---
FROM bellsoft/alpaquita-linux-go:1.26.3-musl AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/fb2cng-web .

# --- Stage 3: hardened runtime ---
FROM bellsoft/hardened-base:musl
COPY --from=fbc /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=fbc /opt/fbc /usr/local/bin/fbc
COPY --from=build /out/fb2cng-web /usr/local/bin/fb2cng-web
ENV FBC_BIN=/usr/local/bin/fbc PORT=8080 TMPDIR=/tmp
EXPOSE 8080
USER 65532:65532
ENTRYPOINT ["/usr/local/bin/fb2cng-web"]
```

---

### Task 1: Pre-flight — baseline build and unit tests still pass on the current Dockerfile

Establish a known-good baseline before changing anything.

**Files:**
- Read only: `Dockerfile`

- [ ] **Step 1: Run the Go unit tests**

Run: `go test ./...`
Expected: PASS (all packages ok).

- [ ] **Step 2: Build the current image as a baseline**

Run: `docker build -t fb2cng-web:baseline .`
Expected: build succeeds. Note the image size from `docker images fb2cng-web:baseline` for later comparison.

- [ ] **Step 3: Smoke-test the baseline container**

```bash
docker rm -f fb2cng-smoke 2>/dev/null || true
docker run -d --name fb2cng-smoke -p 8080:8080 fb2cng-web:baseline
sleep 2
curl -fsS http://localhost:8080/defaults | head -c 200; echo
curl -fsS -o /tmp/out.epub -F file=@testdata/sample.fb2 http://localhost:8080/convert
ls -l /tmp/out.epub
docker rm -f fb2cng-smoke
```
Expected: `/defaults` returns YAML, `/convert` writes a non-empty `out.epub`. This confirms the smoke procedure works before we change the base images.

- [ ] **Step 4: Commit the plan (no code change yet)**

```bash
git add docs/superpowers/plans/2026-06-06-alpaquita-container-migration.md
git commit -m "docs: add alpaquita container migration plan"
```

---

### Task 2: Migrate Stage 1 (fbc fetch) to Alpaquita base

Swap `alpine:3.20` → `bellsoft/alpaquita-linux-base:stream-musl`. Add `ca-certificates` to the `apk add` so the bundle is present for Stage 3 to copy.

**Files:**
- Modify: `Dockerfile:4` and `Dockerfile:7`

- [ ] **Step 1: Change the Stage 1 base image**

Replace line 4:
```dockerfile
FROM alpine:3.20 AS fbc
```
with:
```dockerfile
FROM bellsoft/alpaquita-linux-base:stream-musl AS fbc
```

- [ ] **Step 2: Add ca-certificates to the apk install**

Replace the `RUN apk add` line (line 7) so the package list reads:
```dockerfile
RUN apk add --no-cache curl unzip ca-certificates \
```
(Keep the rest of the `&& curl ... && unzip ... && chmod +x /opt/fbc` continuation unchanged.)

- [ ] **Step 3: Build only Stage 1 to verify the base swap**

Run: `docker build --target fbc -t fb2cng-fbc:wip .`
Expected: build succeeds; `apk add` installs `curl unzip ca-certificates`; the `fbc` binary downloads and `/opt/fbc` is created.

- [ ] **Step 4: Verify the CA bundle exists in Stage 1 (needed by Stage 3)**

Run: `docker run --rm fb2cng-fbc:wip ls -l /etc/ssl/certs/ca-certificates.crt`
Expected: file exists and is non-empty.

- [ ] **Step 5: Commit**

```bash
git add Dockerfile
git commit -m "build: migrate fbc fetch stage to alpaquita-linux-base (musl)"
```

---

### Task 3: Migrate Stage 2 (Go build) to Alpaquita Go image

Swap `golang:1.26` → `bellsoft/alpaquita-linux-go:1.26.3-musl`. The build command is unchanged (`CGO_ENABLED=0`).

**Files:**
- Modify: `Dockerfile:14`

- [ ] **Step 1: Change the Stage 2 base image**

Replace line 14:
```dockerfile
FROM golang:1.26 AS build
```
with:
```dockerfile
FROM bellsoft/alpaquita-linux-go:1.26.3-musl AS build
```

- [ ] **Step 2: Build only Stage 2 to verify the Go toolchain and compile**

Run: `docker build --target build -t fb2cng-build:wip .`
Expected: `go mod download` and `CGO_ENABLED=0 go build -o /out/fb2cng-web .` both succeed.

- [ ] **Step 3: Verify the built binary is statically linked (musl-safe)**

Run: `docker run --rm fb2cng-build:wip sh -c "go version && file /out/fb2cng-web"`
Expected: Go 1.26.x reported; `file` shows `statically linked` (CGO disabled). A static binary runs on the hardened musl base regardless of libc.

- [ ] **Step 4: Commit**

```bash
git add Dockerfile
git commit -m "build: migrate go build stage to alpaquita-linux-go:1.26.3-musl"
```

---

### Task 4: Migrate Stage 3 (runtime) to hardened Alpaquita base

Swap `debian:bookworm-slim` → `bellsoft/hardened-base:musl`. Replace the `apt-get install ca-certificates` (no package manager in hardened base) with an explicit `COPY` of the CA bundle from Stage 1. Switch `USER nobody` → numeric `USER 65532:65532` and set `TMPDIR=/tmp`.

**Files:**
- Modify: `Dockerfile:22-30` (the entire runtime stage)

- [ ] **Step 1: Replace the runtime stage**

Replace the current Stage 3 block:
```dockerfile
# --- Stage 3: runtime ---
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates \
 && rm -rf /var/lib/apt/lists/*
COPY --from=fbc /opt/fbc /usr/local/bin/fbc
COPY --from=build /out/fb2cng-web /usr/local/bin/fb2cng-web
ENV FBC_BIN=/usr/local/bin/fbc PORT=8080
EXPOSE 8080
USER nobody
ENTRYPOINT ["/usr/local/bin/fb2cng-web"]
```
with:
```dockerfile
# --- Stage 3: hardened runtime ---
FROM bellsoft/hardened-base:musl
COPY --from=fbc /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=fbc /opt/fbc /usr/local/bin/fbc
COPY --from=build /out/fb2cng-web /usr/local/bin/fb2cng-web
ENV FBC_BIN=/usr/local/bin/fbc PORT=8080 TMPDIR=/tmp
EXPOSE 8080
USER 65532:65532
ENTRYPOINT ["/usr/local/bin/fb2cng-web"]
```

- [ ] **Step 2: Build the full image**

Run: `docker build -t fb2cng-web:alpaquita .`
Expected: all three stages build; no `apt`/`apk` runs in Stage 3 (it has none).

- [ ] **Step 3: Confirm the runtime user is non-root**

Run: `docker inspect -f '{{.Config.User}}' fb2cng-web:alpaquita`
Expected: `65532:65532`.

If this UID cannot read/execute the copied binaries or write `/tmp` (caught in Task 5), the fix is to add, before the `USER` line, ownership on the temp dir — e.g. `COPY --from=fbc --chown=65532:65532 /tmp /tmp` is NOT valid (empty); instead rely on the base's world-writable `/tmp` (mode 1777) verified in Task 5 Step 2, or mount a tmpfs (Task 6). Do not add a `RUN` — the hardened base has no shell.

- [ ] **Step 4: Commit**

```bash
git add Dockerfile
git commit -m "build: migrate runtime to hardened alpaquita base (musl), non-root 65532"
```

---

### Task 5: Full container smoke test on the hardened image

Prove the fbc binary runs on the hardened musl base, that `/defaults` and a real `/convert` both work, and that `/tmp` is writable by UID 65532.

**Files:**
- Read only: `testdata/sample.fb2`

- [ ] **Step 1: Start the hardened container**

```bash
docker rm -f fb2cng-hardened 2>/dev/null || true
docker run -d --name fb2cng-hardened -p 8080:8080 fb2cng-web:alpaquita
sleep 2
docker logs fb2cng-hardened
```
Expected: log line `fb2cng-web listening on :8080 (fbc=/usr/local/bin/fbc, auth=false)`; no crash/exit.

- [ ] **Step 2: Exercise fbc + /tmp via a real conversion**

```bash
curl -fsS http://localhost:8080/defaults | head -c 200; echo
curl -fsS -o /tmp/out-hardened.epub -F file=@testdata/sample.fb2 http://localhost:8080/convert
ls -l /tmp/out-hardened.epub
```
Expected: `/defaults` returns fbc's YAML defaults (proves the fbc binary executes on the hardened base); `/convert` returns a non-empty EPUB (proves `os.MkdirTemp` could write `/tmp` as UID 65532 and the conversion ran end-to-end).

If `/convert` fails with a temp-dir permission/ENOENT error, `/tmp` is not writable for the non-root user → proceed to Task 6's tmpfs mount and re-run this step with `--tmpfs /tmp:rw,mode=1777` added to the `docker run` in Step 1.

- [ ] **Step 3: Confirm static files still serve**

Run: `curl -fsS http://localhost:8080/ | head -c 100; echo`
Expected: HTML from `internal/web/index.html`.

- [ ] **Step 4: Compare image size to baseline**

```bash
docker images --format '{{.Repository}}:{{.Tag}} {{.Size}}' | grep -E 'fb2cng-web:(baseline|alpaquita)'
docker rm -f fb2cng-hardened
```
Expected: `fb2cng-web:alpaquita` is present; record sizes (hardened musl is expected to be smaller than the debian-slim baseline).

- [ ] **Step 5: Commit (no code change; this task is verification only)**

No commit needed if Tasks 2–4 already committed. If any fix was applied here, commit it:
```bash
git add Dockerfile
git commit -m "build: fix hardened runtime smoke-test findings"
```

---

### Task 6: Document tmpfs + update README/compose to reflect the migration

Make the writable-`/tmp` requirement explicit for hardened-base operators, and update docs that reference the old base.

**Files:**
- Modify: `docker-compose.example.yml`
- Modify: `README.md`

- [ ] **Step 1: Add a tmpfs mount to the fb2cng-web service**

In `docker-compose.example.yml`, under the `fb2cng-web:` service, add a `tmpfs` entry so the hardened, read-root container always has a writable temp dir for conversions. The service block becomes:
```yaml
  fb2cng-web:
    build: .
    environment:
      AUTH_FORWARD_AUTH: "true"
      TRUSTED_PROXIES: ""   # optional: set to caddy's container IP for defense-in-depth
    tmpfs:
      - /tmp:rw,mode=1777
    expose:
      - "8080"
    # No "ports:" — never expose the app directly when auth is on.
```

- [ ] **Step 2: Validate the compose file parses**

Run: `docker compose -f docker-compose.example.yml config >/dev/null && echo OK`
Expected: `OK` (no YAML/schema errors).

- [ ] **Step 3: Note the hardened base in README**

In `README.md`, under the `## Run` section, after the `docker run` line, add:
```markdown
> The image is built on BellSoft Alpaquita Linux (musl); the production runtime stage uses
> the hardened Alpaquita base (`bellsoft/hardened-base:musl`) — minimal, non-root (UID 65532),
> no shell or package manager. The app writes conversion temp files to `/tmp`, so when running
> a read-only root filesystem, mount a writable `/tmp` (the compose example uses `tmpfs`).
```

- [ ] **Step 4: Commit**

```bash
git add docker-compose.example.yml README.md
git commit -m "docs: document hardened alpaquita base and writable /tmp requirement"
```

---

### Task 7: Final verification

- [ ] **Step 1: Clean rebuild from scratch**

Run: `docker build --no-cache -t fb2cng-web:alpaquita .`
Expected: all three Alpaquita stages build cleanly with no cache.

- [ ] **Step 2: Re-run unit tests**

Run: `go test ./...`
Expected: PASS (the migration touches only the Dockerfile/docs, not Go code — this confirms nothing drifted).

- [ ] **Step 3: One final container smoke**

```bash
docker rm -f fb2cng-final 2>/dev/null || true
docker run -d --name fb2cng-final --tmpfs /tmp:rw,mode=1777 -p 8080:8080 fb2cng-web:alpaquita
sleep 2
curl -fsS -o /tmp/final.epub -F file=@testdata/sample.fb2 http://localhost:8080/convert && echo "convert OK"
docker rm -f fb2cng-final
```
Expected: `convert OK` and a non-empty `/tmp/final.epub`.

- [ ] **Step 4: Confirm no `alpine`, `golang:`, or `debian` references remain**

Run: `grep -nE 'alpine|golang:|debian' Dockerfile || echo "clean"`
Expected: `clean` (every base image is now `bellsoft/...`).
```

---

## Self-Review

**Spec coverage:**
- "migrate all containers to alpaquita" → Tasks 2 (fetch stage), 3 (build stage), 4 (runtime stage) cover all three stages we control; out-of-scope upstream images (authelia/caddy) explicitly documented in Scope.
- "Production one should be alpaquita hardened" → Task 4 puts the runtime (production) stage on `bellsoft/hardened-base:musl` and verifies non-root in Task 5.

**Placeholder scan:** No TBD/TODO/"add error handling" placeholders. Every code/Dockerfile change is shown in full.

**Type/identifier consistency:** Image tags consistent throughout — fetch `bellsoft/alpaquita-linux-base:stream-musl`, build `bellsoft/alpaquita-linux-go:1.26.3-musl`, runtime `bellsoft/hardened-base:musl`. Runtime user `65532:65532` used identically in the Dockerfile, `docker inspect` check, and README. Smoke endpoints (`/defaults`, `/convert`, `/`) match `internal/server/server.go`. CA bundle path `/etc/ssl/certs/ca-certificates.crt` consistent between Stage 1 verify (Task 2) and Stage 3 COPY (Task 4).
