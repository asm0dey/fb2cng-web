# Editor option sub-sections

## Problem

The preset option editor groups options by a flat `group` field. The `document`
group holds 45 options in a single accordion. Nested options like
`document.images.optimize` are buried in that flat list — non-obvious and hard
to find, because the natural sub-structure already present in the key path
(`document.images.*`, `document.footnotes.*`, …) is discarded in the UI.

## Goal

Split each group into collapsible sub-sections derived from the option key path,
so `document.images.optimize` lives under an `Images` section rather than loose
under `document`.

## Decisions

- **Section source:** auto-derived from the key path — no schema changes.
- **Rendering:** nested accordion (`<details>` inside the group `<details>`).
- **Scope:** applied to all groups uniformly.
- **Default state:** sub-sections start collapsed when a group is expanded.
- **Labels:** section headers are prettified (`text_transformations` →
  `Text transformations`). Group names stay raw (out of scope, avoids churn).

## Section-derivation rule

For each option key, split on `.`:

- `≥ 3` segments → section = 2nd segment (`document.images.optimize` → `images`).
- exactly 2 segments → section = `general` (`document.fix_zip` → `general`).

Same rule applies to synthetic override rows (unknown keys). Sections keep
schema first-seen order. For `document` that yields:

```
general, metainformation, page_map, images, footnotes,
annotation, toc_page, dropcaps, text_transformations
```

(`general` is first because the bare `document.*` options appear first in schema
order.)

Prettify label: replace `_` with space, capitalize first letter. `general`
stays `General`.

## Data model (`internal/server/editor.go`)

Replace `groupVM.Rows` with sections:

```go
type sectionVM struct {
    Name    string // raw key segment (data-section, search)
    Label   string // prettified header
    Total   int
    Changed int
    Rows    []rowVM
}

type groupVM struct {
    Name     string
    Total    int
    Changed  int
    Flat     bool         // len(Sections) == 1 → render rows directly
    Sections []sectionVM
}
```

- Group `Total`/`Changed` = sum over its sections (keeps existing group badges).
- `Flat` collapses trivial groups (`version`, `reporting`, any single-section
  group) so they render rows directly with no pointless nested accordion.

Build path: `appendSchemaRows` / `appendSyntheticRows` route each row into a
section within its group (preserving first-seen order for both groups and
sections), then `finalize` computes section, group, and total counts and sets
`Flat`.

## Template (`internal/web/templates/editor.gohtml`)

Extract the option-row markup into `{{define "optrow"}}` so it renders
identically in flat and nested contexts (no duplication).

Group loop:

```
{{range .Groups}}
<details class="opt-group{{if gt .Changed 0}} has-changes{{end}}" data-group="{{.Name}}">
  <summary class="opt-group-sum">… {{.Name}} … {{.Total}} · {{.Changed}} changed</summary>
  {{if .Flat}}
    {{range (index .Sections 0).Rows}}{{template "optrow" .}}{{end}}
  {{else}}
    {{range .Sections}}
    <details class="opt-section{{if gt .Changed 0}} has-changes{{end}}" data-section="{{.Name}}">
      <summary class="opt-section-sum">
        <span class="caret">▸</span>
        <span class="opt-section-name">{{.Label}}</span>
        <span class="opt-section-count">{{.Total}} · <span class="sec-changed">{{.Changed}}</span> changed</span>
      </summary>
      {{range .Rows}}{{template "optrow" .}}{{end}}
    </details>
    {{end}}
  {{end}}
</details>
{{end}}
```

Nested `<details>` have no `open` attribute → collapsed by default.

## JavaScript (inline in `editor.gohtml`)

- **`apply()`** (search + changed-only filter): for each group, walk its
  `.opt-section`s, count matching `.option-row`s per section, hide empty
  sections, and auto-`open` sections that have matches while filtering. Flat
  groups have rows as direct children — handle those directly. Group is shown
  and auto-opened when any of its rows match, as today.
- **`recount()`**: in addition to the existing per-group and total counters,
  update each section's `.sec-changed` badge and toggle its `has-changes` class.
- **`collapse-all`**: unchanged — toggles top-level `.opt-group`s only. Sections
  are hidden while their group is collapsed, and return to their default
  (collapsed) state on expand.

## CSS (`internal/web/static/app.css`)

- `.opt-section` indented under the group.
- `.opt-section-sum` mirrors `.opt-group-sum`, lighter weight, to read as a
  subordinate header.
- Reuse the existing `.caret` open/close rotation for the section carets.

## Tests

- `internal/server/editor_test.go`:
  - `document.images.optimize` lands in an `images` section.
  - Bare `document.*` options land in `general`.
  - A single-option group has `Flat == true`.
  - Section `Total`/`Changed` counts sum correctly to the group.
- `internal/web/templates_test.go`: rendered editor contains a section summary
  (e.g. `Images`) nested inside the `document` group.

## Out of scope

- Prettifying group names.
- Changing the section grouping to an explicit schema field.
- Further nesting beyond one section level (e.g. `metainformation.*` sub-keys).
