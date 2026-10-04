# AI Instructions

Toki is an i18n (internationalization) framework for Go. See [README.md](README.md) for more information.

## General Engineering

- **Avoid unnecessary memory allocations** — prefer avoiding them when it doesn't add significant complexity or code volume (e.g. `strings.Builder` over `+=` in loops). One example where more allocations are fine is using `json.Marshal` over manual serialization because that greatly reduces code volume and code complexity.
- **Remove dead code** — don't leave unused functions, types, or imports. If something becomes unused after a refactor, delete it.
- **DRY (Don't Repeat Yourself)** — when multiple handlers share the same pattern, factor the common code into a helper rather than duplicating it.
- **Separation of concerns** — each type, function, and package should have a single clear responsibility. Don't mix unrelated state into shared structs (e.g. different page types should have separate state types, not a single "view state" with fields tagged "only used by X"). Don't put presentation logic in data-layer code or vice versa. When a function starts doing two things, split it. When a struct serves two masters, split it.
- **Explicit data flow** — state should flow down explicitly through parameters, attributes, and arguments — not tunnel implicitly through events, global listeners, or shared mutable state. When a component needs data, pass it directly rather than having the component listen for and react to ambient signals. Implicit wiring is fragile: it hides dependencies, breaks when intermediaries change, and makes code hard to trace. If you can't answer "where does this value come from?" by reading the call site, the flow is too indirect.
- **YAGNI** — solve the actual problem, not a hypothetical generalized version of it. Keep solutions proportional to the problem.
- **Watch for accidental complexity** — before adding a new abstraction, layer, or mechanism, ask whether the problem can be solved with what already exists. For example, don't introduce client-side signal manipulation when a server POST action achieves the same result. Don't add a wrapper component when an attribute on an existing element suffices. Every indirection has a cost — if the simpler path works, take it, unless there's a good reason not to.
- **Never suppress errors or warnings** — do not add `nolint`, `//datapages:nolint`, or similar suppression directives unless the user explicitly asks for it. Errors and warnings exist for a reason — fix the underlying issue instead of silencing it. If there's no other way but to suppress the error then ask the user first and state the reason.
- **Never edit generated files**: `*_templ.go`, `*_gen.go`, and any file with the "DO NOT EDIT" header comment. Only edit source files owned by the user.

## Editor

The following instructions apply to the Toki editor under `editor/`.

The editor is a [Datapages](https://github.com/romshark/datapages) v0.10.1 application. Follow the Datapages and Datastar skills in `.claude/skills/`, written by `datapages init`: `datapages-architecture` before designing a feature, `datapages` for any editor work, and the task skill it points to. This section only records what is specific to Toki. Where it differs from the skills, this section wins.

- **Layout**: the app package is `editor/app` and its generated code is `editor/app/datapagesgen`. `editor/editor.go` holds the `datapages.NewServer` call. `cmd/editor-server` is the entry point `datapages watch` runs.
- **Never run `templ generate`**, whether a watch runs or not: the user always runs `datapages watch`, which regenerates the templates.
- **Run `datapages gen`** to check for compilation errors and lint feedback. Don't use `datapages lint`.

### Architecture

The editor uses the skills' default architecture: CQRS with fat morphs, where every `OnXXX` handler re-renders its whole page from current state. On top of that:

- **Echo suppression**: The editor pages' `POSTSet` actions put their tab's `stateID` on `EventUpdated`. `OnUpdated` compares it and leaves the source tab's changed editor out of its `$editor` patch, which keeps the tab's own echo from overwriting what is being typed. A shared `*App` action cannot take state for pages with different state types, which is why each editor page declares its own `POSTSet`.
- **UI preferences** (theme, fonts) live in cookies read by `ReadUIPrefs`, not in server state. `POSTSetPref` dispatches `EventPrefsChanged`, which every page handles through the embedded `PrefsSync`.
- **Avoid `templ.Raw`**: Prefer templ's native constructs over building raw HTML strings whenever possible.
- **Offline-capable**: All assets (CSS, JS) are embedded static files — no CDN dependencies. The app must work offline. This includes Datastar itself (`static/datastar.js`, set via `datapages.WithDatastarJS`), which must match the version the Datapages release loads by default.
- **Morph safety**: Elements that persist across morphs need stable `id` attributes for Datastar to match old and new elements. A morph re-applies every `data-signals` attribute whose value changed. Seed page signals the client owns (typed input, editor text) with `__ifmissing` and patch the ones the server owns before morphing. Editable `<toki-editor>` instances carry `data-preserve-attr="value"`: their value follows `$editor`, not the rendered markup. Use `data-ignore-morph` only when that is not enough: marked elements are invisible to the server's updates, which creates stale UI and complexity.
- **Web components for rich client-side widgets**: Use custom elements (`editor/js/wc/`) for functionality that requires complex client-side state management and rendering (e.g. `<toki-editor>` for ICU message editing with CodeMirror). These are the exception to the server-first rule — they encapsulate self-contained interactive widgets. Web components receive inputs from the server via Datastar signals and attributes (e.g. `data-attr:theme`, `data-bind:value`), keeping the server in control of what the component displays and how it behaves.

### UI Elements

The editor uses [Morpheus](https://github.com/romshark/morpheus) (`github.com/romshark/morpheus`), a web-component UI kit built for server-driven Go + Templ + Datastar stacks. Its assets are vendored into `editor/app/static/` (`morpheus.css`, `theme-default.css`, `morpheus.js`) to keep the editor offline-capable. When building or modifying the editor UI:

- **Prefer Morpheus components** (alert, badge, card, button, select, switch, sidebar, pagination, tooltip, etc.) over custom CSS whenever possible. Use the typed Templ wrappers from `github.com/romshark/morpheus/neo` — `neo.Button(neo.ButtonOpts{...})` — rather than hand-writing `<neo-*>` tags.
- **Pass Datastar bindings through the `*Attrs` variants** — every component has a `XxxAttrs(opts, templ.Attributes)` form for `data-on:*`, `data-bind`, `id`, and classes. Merge attribute sets with the local `attrs(...)` helper.
- **Check which events a component emits before reaching for `data-bind`.** `<neo-textinput>` dispatches native `input`/`change`, so `data-bind` works. `<neo-select>`, `<neo-switch>`, and `<neo-pagination>` report state only through their own events (`neo-select-change`, `neo-switch-change`, `neo-pagination-change`), so assign the signal from `evt.detail` inside an `action.WithBefore(...)` expression instead.
- **Omit boolean command attributes to preserve client state.** Under Morpheus's command-attribute contract an absent boolean (e.g. `open` on `<neo-sidebar>`) means "keep current state", which is what makes fat morphs safe; setting `Open` explicitly forces the state on every patch.
- **Respect the slot contracts.** `neo.CardHeader`/`CardBody`/`CardFooter` and `neo.SidebarHeader`/`SidebarContent()`/`SidebarFooter` must be authored as immediate children of their host. Note the two differ in the rendered DOM: sidebar slots stay direct children of `<neo-sidebar>`, but card slots are wrapped in a `<div data-neo-card-inner>`, so app CSS targeting them must use a **descendant** selector (`.my-card [data-neo-card-body]`), never `>`. Card slots are also spaced by `[data-neo-card-inner] > * + *`, so any extra element placed inside the card (an absolutely-positioned overlay, say) shifts the first slot out of first position and gives it a stray top margin — put such elements outside the card.
- **Use modern nested CSS** — group related styles using CSS nesting instead of flat selectors. This keeps styles co-located with their parent context and reduces repetition.
- **Use Morpheus's design tokens** — `--page-bg`, `--page-fg`, `--accent`, `--accent-fg`, `--btn-border`, `--btn-hover-bg`, `--danger-bg`, `--muted`. Per-component knobs are namespaced `--neo-<component>-*`.
