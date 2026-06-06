# fb2cng Web Interface — Design

**Date:** 2026-06-06
**Status:** Approved design, pre-implementation

## Summary

A web interface for [fb2cng](https://github.com/rupor-github/fb2cng) (`fbc`), a Go CLI that
converts FictionBook (fb2) files to EPUB and other e-book formats. The web app provides a
drop area for `fb2` / `fb2.zip` files, converts them by invoking the bundled `fbc` binary,
and auto-downloads each result. It uses fbc's embedded defaults out of the box, and lets the
user override settings either through a small form of common options or by uploading / editing
raw YAML.

## Scope & Decisions

| Topic | Decision |
|-------|----------|
| Deployment | Self-hosted for a few users (home server / small VPS). Light concurrency. No public-internet abuse hardening. |
| Architecture | Single **Go service** that shells out to the bundled `fbc` binary. fbc's CLI is the stable contract. |
| Config UI | **Common-options form + raw YAML editor.** Upload of full YAML supported. Not a full per-field form. |
| Batch I/O | **Multi-file in**; each result **auto-downloads** on completion. No persistent download links, no result storage. |
| Output formats | Selector exposes **all fbc formats** (epub2, epub3, kepub, kfx, azw8, pdf). **Default epub3.** |
| Layout | **Single column, settings collapsed** behind a toggle (defaults just work). |
| Responsive | Mobile-friendly; drop area doubles as tap-to-pick file input. |
| Theming | Light + dark, default follows system, manual toggle persisted to `localStorage`. |

## Architecture

Single Go service, packaged as **one Docker image** bundling the `fbc` binary.

### Components

- **HTTP server** — serves the static frontend and exposes:
  - `GET /defaults` — returns fbc's default config YAML (from `fbc dumpconfig --default`), used to
    seed the editor and read default values into the form.
  - `POST /convert` — accepts one file + chosen format + effective config; returns the converted
    file as a download (or a per-file error).
- **Conversion handler** — per uploaded file, stateless:
  1. Create an isolated temp workdir.
  2. Write the input file.
  3. Write the effective config YAML (only if the user supplied overrides).
  4. Run `fbc convert --to <fmt> [-c config.yaml] <input> <dest>/`.
  5. On success, stream the produced file back so the browser auto-downloads it.
  6. Always delete the temp workdir (success or failure).
- **Config builder** — produces the effective YAML (see Config Handling).
- **Frontend (vanilla JS)** — drop area, collapsible settings (common form + raw YAML textarea),
  per-file progress/status list, theme toggle. No framework.
- **Concurrency guard** — a small worker pool (N = 2–4) so a large multi-file drop does not spawn
  unlimited `fbc` processes.

### Data flow

```
drop files
  └─ for each file: POST /convert (file + format + effective config)
        └─ server: temp workdir → write input + config → run fbc → stream output back → cleanup
              └─ browser: auto-download result  (or show per-file error)
```

Nothing is persisted between requests; the server holds no state.

## Config Handling

- On load, the frontend fetches `GET /defaults` and uses fbc's default YAML to seed the raw-YAML
  editor and populate the common form fields.
- **Effective config** is built as:
  1. **Base** = the raw YAML the user edited or uploaded (empty if untouched).
  2. **Overlay** = the common-form values, which **win** on any conflict.
  - If the user touched neither form nor YAML, send **no config** → fbc uses its embedded defaults.
- "Upload settings" = pick/drop a `.yaml` file → loads into the raw editor and refreshes the form
  fields it recognizes.
- fbc itself merges the provided config over its embedded defaults, so the app only ever needs to
  send the overrides.

### Common-options form (the handful of common knobs)

- Output format → `--to` (epub2/epub3/kepub/kfx/azw8/pdf; default epub3)
- ToC type (`document.toc_type`: normal / old_kindle / flat)
- Image optimize on/off + JPEG quality (`document.images.optimize`, `jpeg_quality_level`)
- Footnote mode (`document.footnotes.mode`: default / float / floatRenumbered)
- Soft-hyphen insertion (`document.insert_soft_hyphen`)

Everything else is reachable through the raw YAML editor.

## Error Handling

- Reject non-`fb2` / non-`zip` inputs before upload.
- On `fbc` non-zero exit: capture stderr, show a short message plus expandable detail in that file's
  status row.
- Invalid YAML: caught early with a clear message (optionally validated via `fbc dumpconfig`).
- Temp workdir is always cleaned, including on failure.
- **Browser multi-download caveat:** browsers may prompt to "allow multiple automatic downloads"
  for a large batch. Acceptable for self-hosted use.

## Responsive & Theming

- **Mobile:** single-column layout reflows cleanly; drop area doubles as a tap target opening the
  native file picker (drag-drop is weak on touch); settings and YAML editor stack full-width with
  tap-sized controls.
- **Themes:** light + dark via CSS custom properties. Default follows `prefers-color-scheme`; a
  header toggle (light / dark / system) persists to `localStorage`. Theme is applied before first
  paint to avoid flash.

## Testing

- Go handler tests against sample `fb2` and `fb2.zip` fixtures: success, corrupt fb2, bad config,
  multi-file.
- Config-merge unit tests: form overlay over raw YAML; untouched → no config sent.
- Smoke test: a produced epub opens / passes `epubcheck` when available.

## Non-Goals

- No public-internet abuse hardening (rate limits, quotas, isolation beyond temp dirs).
- No user accounts, history, or stored conversion results.
- No full per-field config form (raw YAML covers the long tail).
- No in-browser WASM conversion (fbc runs as a subprocess on the server).
