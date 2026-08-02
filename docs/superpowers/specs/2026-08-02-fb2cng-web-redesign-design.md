# fb2cng Web — Full Redesign (Convert flow, Presets, Option editor)

**Date:** 2026-08-02
**Status:** Approved design, pre-implementation
**Supersedes (partially):** `2026-06-06-fb2cng-web-interface-design.md` — reverses its
stateless / no-persistence / no-full-form / Pico-CSS decisions.

## Summary

Rebuild the fb2cng web UI to match the approved mockup (`FB2 Converter.dc.html`). The app
moves from a stateless single-page app (Pico CSS + vanilla JS, stream-and-forget conversion)
to a **server-rendered, stateful** application: `html/template` pages with htmx for the
convert-card region, a **named preset store**, a **retained job store** (results + per-file
logs + retry), and a **full per-option settings editor** driven by a checked-in option schema.

Still one Go service that shells out to the bundled `fbc` binary; fbc's CLI stays the stable
contract. Deployment remains self-hosted for a few users with optional forward-auth.

## Locked Decisions

| Topic | Decision |
|-------|----------|
| Scope | **Full fidelity** — Convert flow + Presets + full ~104-option editor + retained results/logs/retry. |
| Rendering | **Server-rendered `html/template` + htmx** (CDN). Real `<form>` POSTs, `<details>` for collapse; htmx swaps only the convert-card / effective-config regions. |
| Styling | The mockup's own CSS: 8 CSS custom properties, one breakpoint at 640px, IBM Plex Mono + Helvetica Neue (Google Fonts). **Pico CSS dropped.** |
| Theme | Dark via `@media (prefers-color-scheme: dark)` re-declaring the 8 props, **plus** a retained manual light/dark/system toggle (localStorage), applied before first paint. |
| Presets | Named, persisted, single **shared** store (no per-user data). CRUD + one default marker. |
| Option editor | Full grid of every fbc option, grouped, with type-appropriate inputs, per-option reset, changed markers, search, "changed only" filter, live effective-config YAML + validity. |
| Option schema | Checked-in `options.json`. Scaffolded from `fbc dumpconfig --default` (keys/types/defaults) + descriptions/enums from `docs/config.md` and hand-fill. |
| Jobs | A batch conversion = one job dir holding inputs, outputs, per-file logs, status. TTL sweeper. Enables results view, download-all zip, stored logs, retry. |
| Storage | **Split dirs:** `PRESETS_DIR` (persistent volume) + `JOBS_DIR` (ephemeral-until-TTL). |
| Auth | Unchanged — optional forward-auth, off by default; existing middleware reused. |
| Reused | `convert.BuildConfig` layering, `convert.FBC` runner, forward-auth middleware, concurrency sem, format allowlist. |

## Architecture

Single Go service, one Docker image bundling `fbc`. Pages are server-rendered; htmx performs
partial swaps for the two dynamic regions (convert-card during/after a job, effective-config
pane in the editor). No SPA framework. `app.js` shrinks to glue: drag-drop → assign the file
input, theme toggle, and a couple of htmx helpers.

### New packages

- **`internal/schema`** — loads `options.json` at startup into an ordered, grouped list of
  option descriptors: `{key, group, label, type (bool|int|string|enum), default, enum?,
  description?}`. `type`/`default` come from the scaffold; `label`/`description`/`enum` are
  annotated. Exposes lookups used by the editor renderer and by change-detection
  (value == default?). A `cmd/schemagen` (run via `go generate`) regenerates the scaffold from
  `dumpconfig --default` + `docs/config.md` and reports keys that drifted (added/removed/retyped)
  so the maintainer knows what to re-annotate after an fbc bump.

- **`internal/presets`** — CRUD over `PRESETS_DIR`. One file per preset:
  `PRESETS_DIR/<id>.yaml`, where the body is a **sparse override map** (only keys that differ
  from fbc defaults), the same shape `BuildConfig` already consumes. Metadata (display name,
  is-default, updated-at, changed-count) lives in the file's header keys or a sidecar; a single
  `default` marker is stored once (e.g. a `default` symlink or a `_meta.yaml`). "Defaults" is a
  synthetic read-only built-in preset (empty override map), never written to disk.

- **`internal/jobs`** — a batch = `JOBS_DIR/<id>/` containing: the uploaded inputs, an `out/`
  dir per input, a captured `*.log` per input (fbc stdout+stderr), and a `status.json`
  (per-file: state, output names, sizes, log line count, first-error line, timing). A background
  sweeper deletes job dirs older than a TTL (env-configurable). The job store is what makes the
  results view, download-all zip, stored logs, and retry possible.

### Routes

Server-rendered pages:
- `GET /` — Convert tab (idle / file-selected states are one template toggled by an `.is-empty`
  class; no separate template).
- `GET /settings` — Presets list.
- `GET /settings/preset/{id}` — Preset editor.

htmx / actions:
- `POST /convert` — parse multipart (files + preset id), create a job, kick off conversion
  (existing sem cap), return the convert-card partial. htmx polls status.
- `GET /jobs/{id}` — convert-card partial reflecting current status (running → done/failed).
- `GET /jobs/{id}/download/{file}` — stream one output.
- `GET /jobs/{id}/zip` — stream all outputs as one zip ("Download all").
- `GET /jobs/{id}/log/{file}` — full captured log (for the `<details>` / copy).
- `POST /jobs/{id}/retry` — re-run the failed input(s) with one option overridden (e.g.
  `use_broken_images=true`); reuses the retained inputs. Returns updated convert-card partial.
- `POST /settings/preset` — create; `GET/POST /settings/preset/{id}` — read/update;
  `POST /settings/preset/{id}/delete`; `POST /settings/preset/{id}/duplicate`;
  `POST /settings/preset/{id}/default` — set as default.
- `POST /settings/preset/{id}/effective` — takes the current editor field values, merges over
  fbc defaults via `BuildConfig`, returns the effective-config YAML partial + a validity
  indicator (valid/invalid, override count). Validity is checked by having `fbc` parse the
  merged config (a `dumpconfig -c <merged>` style probe); invalid → surfaced inline.

### Data flow

```
Convert:
  pick preset + files → POST /convert
    → job dir written; for each input: fbc convert (sem-capped), log captured to <input>.log
    → htmx polls GET /jobs/{id}
        running → progress card
        done    → result rows (per-file Download) + "Download all .zip" + logs <details>
        failed  → error card (message + suggested retry) + first-error <pre>
                  + full-log <details> + [Retry with <option>] [Skip]

Preset editor:
  edit option grid → (on change) POST .../effective → live merged YAML + valid/invalid + N overrides
  Save → write PRESETS_DIR/<id>.yaml (sparse overrides) + metadata
```

## Config / Preset Handling

- **Effective config** layering is unchanged in spirit and reuses `convert.BuildConfig`:
  application defaults < preset overrides < (any per-convert override, e.g. retry flip). fbc
  merges the sent config over its own embedded defaults, so only overrides are ever sent.
- A preset stores **only** keys whose value differs from the fbc default (sparse). Change
  detection in the editor compares each field to `schema` default; equal → drop the key, clear
  the amber "changed" marker.
- "Defaults" built-in preset = empty override map, read-only, cannot be deleted or edited.
- Selecting a preset as **default** preselects it on the Convert tab.

## Option Editor (the full grid)

- Rendered from `internal/schema`, grouped by top-level section (document, images, output,
  screen, cover, footnotes, metainformation, …) in schema order.
- Input widget by `type`: checkbox (bool), number (int), select (enum), text/textarea (string;
  templates like `output_name_template` get a textarea + a live filename preview).
- Each row shows: key, current control, description + `default <x>` + `reset` (when changed).
  Changed rows get an amber left-border + dot (`--changed` token).
- Toolbar: search-filter options, "Changed only" filter, "Default preset" checkbox, Save.
- Right pane: **Effective config** — live merged YAML (near-black `<pre>`, kept dark in both
  themes) + a valid/override-count chip, refreshed on save (and on edit via the effective route).
- Options absent from the schema annotation still render (label = key, no description) so the
  grid is complete even before prose is filled in.

## Styling & Theme

- Port the mockup's inline styles into one stylesheet using the 8 documented tokens
  (`--bg`, `--bg-sunk`, `--line`, `--fg`, `--fg-muted`, `--fg-dim`, `--accent`, `--changed`) on
  `:root`, re-declared inside one `@media (prefers-color-scheme: dark)` block.
- Extra dark rules from the mockup: log `<pre>` keeps its own near-black; success/error tints
  drop to ~0.28 lightness rather than inverting.
- Manual toggle overrides OS via a `data-theme` attribute on `<html>` (light/dark/system),
  persisted to localStorage, applied before first paint. The `@media` block is scoped so the
  attribute wins when set.
- One breakpoint at 640px: phone = single column / stacked; desktop = centered max-width form,
  editor = 2-column grid (option list + effective-config aside).
- Fonts: IBM Plex Mono (UI mono / keys / logs) + Helvetica Neue stack (prose), Google Fonts.

## Error Handling

- Reject non-`fb2` / non-`zip` inputs (existing check).
- Per-file fbc failure: capture full log; status.json records first-error line; results view
  shows the error card with a suggested retry (heuristic: known error → known option, e.g.
  missing-binary → `use_broken_images`). Other files in the batch still succeed.
- Invalid preset/effective config: `fbc` parse probe fails → inline invalid indicator, Save
  blocked with the message.
- Job dirs always swept on TTL; inputs retained only until then (needed for retry).
- Browser multi-download caveat is gone — downloads are explicit links / zip, not auto-triggered.

## Storage & Config (env)

- `PRESETS_DIR` — persistent (default e.g. `/data/presets`). Survives restarts; mount a volume.
- `JOBS_DIR` — job scratch (default e.g. `/tmp/fb2cng-jobs` or a tmpfs). Holds inputs+outputs+logs
  until TTL.
- `JOBS_TTL` — max job age before sweep (default e.g. 1h).
- Existing env unchanged: `PORT`, `FBC_BIN`, `MAX_CONCURRENT`, `AUTH_FORWARD_AUTH`,
  `TRUSTED_PROXIES`.
- Dockerfile / docker-compose example updated: declare the presets volume + jobs dir.

## Testing

- `internal/schema`: scaffold loads; type inference from default value; drift report
  (added/removed/retyped keys) against a fixture dump.
- `internal/presets`: CRUD, sparse-override round-trip, default marker exclusivity,
  read-only built-in "Defaults".
- `internal/jobs`: job lifecycle (create → running → done/failed), status.json shape,
  zip assembly, TTL sweep, retry re-run with an overridden option.
- Effective config: merge + validity (valid and invalid fixtures); override count.
- Handlers: `/convert` creates a job and returns the card; `/jobs/{id}` partials for
  running/done/failed; download / zip / log routes; auth middleware unchanged.
- `testdata/fake-fbc.sh` extended to emit a multi-line log and to honor an override that turns a
  "corrupt" input into a success (to exercise retry).
- Smoke: a produced epub opens / passes `epubcheck` when available (existing).

## Migration Notes

- `internal/web/index.html` + `app.js` are replaced by `html/template` templates + a slimmed
  `app.js`; Pico CDN link removed, htmx CDN link added.
- `handleConvert` is refactored to create a job rather than stream inline; the single-file
  stream path is superseded by job download routes.
- `GET /defaults` stays (used by schemagen and, if useful, the editor).
- The 2026-06-06 spec's Non-Goals "no history / no stored results / no full per-field form" are
  explicitly reversed here; forward-auth and self-hosted, low-hardening posture are retained.

## Non-Goals

- No per-user preset isolation (single shared store; forward-auth only gates access).
- No public-internet abuse hardening beyond current posture (temp dirs, sem cap, TTL).
- No in-browser WASM conversion — fbc stays a server subprocess.
- No persistent conversion history beyond the job TTL window.
- Schema descriptions are best-effort/incremental — not a blocker for shipping the editor.

## Decomposition (implementation plans)

Full fidelity is large; expect the plan(s) to sequence, each shippable:

1. **Visual + Convert flow** — templates, CSS/theme, htmx convert-card, job store, results/logs/
   download-all/retry. (Delivers the whole look + flow; presets use the built-in Defaults only.)
2. **Presets** — preset store + Settings/Presets list + CRUD + default marker + wire into Convert.
3. **Option editor + schema** — schemagen tool, `options.json`, full grid, effective-config pane,
   Save. (Heaviest; content-fill of descriptions is incremental after.)
