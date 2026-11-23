# AGENTS.md

Guidance for coding agents and contributors working on **QEMU Glaze**.

## UI Guidelines — GNOME HIG & GTK 4.x+

All UI styles, layouts, and interactions in `ui/` (`ui/index.html`, `ui/app.css`, `ui/app.js`) **must** follow the [GNOME Human Interface Guidelines (HIG)](https://developer.gnome.org/hig/) using **GTK 4.x+ / libadwaita** patterns as the visual and behavioral reference:

- **Layout & Navigation:**
  - Use an `AdwNavigationSplitView` shell (`.adw-split-view` with `.adw-sidebar`, accessible `.adw-nav-row` items with `aria-current="page"`, `.adw-sidebar-footer` for compact host telemetry, and `.adw-content-view` with a single `.adw-toolbar`).
  - Keep scrollable `.adw-page` containers full-width so scrollbars sit cleanly at the window edge, and wrap page sections in `.adw-clamp` (`.tight` / `.medium` / `.large`) with `.adw-group`, `.boxed-list`, and `.adw-row` (`12px` card radius, `9px` control radius).
  - Ensure all pages, multi-column grids (`.hardware-grid`), and VM cards remain usable down to the minimum window size (`960x680`, `glaze.HintMin`) without horizontal clipping or overlapping controls (use `min-width: 0` on flex/grid children and responsive single-column breakpoints).
- **Color, Contrast & Light/Dark Parity:**
  - Every UI surface, border, code/command preview, log terminal, and semantic state must adapt to both dark and light color schemes via CSS custom properties in `:root`, `@media (prefers-color-scheme: light)`, and `:root[data-os-theme="light"]`.
  - Semantic tokens (`--accent-*`, `--success-*`, `--warning-*`, `--destructive-*`, `--info-*`, `--code-*`, `--terminal-*`) must maintain strong legibility and WCAG AA contrast in **both** light and dark modes. Never hardcode dark-only or light-only backgrounds/foregrounds outside theme tokens.
- **Controls, Typography & Feedback:**
  - Follow GTK 4 / Adwaita widget semantics: `.gtk-btn` (`.suggested-action`, `.destructive-action`, `.flat`, `.circular`, `.pill`), `.adw-switch` for boolean rows, `.gtk-dropdown` for selects, `.chip-btn` inside `.preset-chips`, `.gtk-pill` / `.adw-pill` for compact status badges, inline warning callouts (`.adw-callout-banner`) for missing QEMU tools, `.adw-status-page` for empty states, `AdwAlertDialog`-style confirmation modals (`role="dialog"`, `aria-modal="true"`, `Escape` to dismiss), `.adw-tooltip` for floating tooltips, and `.adw-toast-overlay` for non-blocking transient notifications.
  - Keep all user-facing UI labels concise, scannable, approachable, and free of internal implementation jargon (such as CGo, `purego`, or `app://` protocol details) in **Portuguese (`pt-BR`)**; use monospace (`--font-mono`) for paths, QEMU command previews, and console logs.
  - Use crisp monochrome SVG icons (`currentColor`) rather than emojis or decorative clutter in controls, badges, and warnings.
- **Reusable CSS Kit Preservation:**
  - `ui/app.css` intentionally defines a reusable GTK 4 / libadwaita CSS component kit (including `.adw-clamp.*`, `.adw-card`, `.adw-status-page`, `.gtk-dropdown-wrap`, `.gtk-popover-menu`, `.adw-switch`, `.adw-check-label`, `.adw-radio-label`, `.adw-pill`, `.gtk-btn.circular`, `.gtk-btn.pill`, etc.) meant to be shared across Glaze projects. **Do not delete** unused reusable kit classes when modifying application-specific styles.

## Architecture & Project Structure

- `main.go` — Pure Go (`CGO_ENABLED=0`) desktop entry point using **Glaze** (`WKWebView` / `WebKitGTK` via `purego`), `app://` embedded asset handler, native menus, file dialogs, VM telemetry ticker, and `AppAPI` bindings.
- `vm/types.go` — Shared Go/JSON contracts and configuration validation (`VMConfig`, `VMState`, `SystemInfo`, request/response payloads).
- `vm/manager.go` — Host capability and QEMU binary/firmware detection (`qemu-system-x86_64`, `qemu-system-aarch64`, `qemu-img`, EDK2), command builder, QCOW2 disk creation, process lifecycle, and log streaming.
- `store/store.go` — Encrypted AES-256-GCM local persistence with atomic writes.
- `ui/` — Embedded frontend assets (`ui/index.html`, `ui/app.css`, `ui/app.js`).
- `build_mac.go`, `build_linux.go`, `assets/` — Native macOS `.app` / Linux `.AppImage` packagers and icon generator.

## Tooling & Checks

Always run formatting, static checks, and tests after making changes:

```bash
./fmt.sh
go vet ./...
go test -count=1 ./...
```
