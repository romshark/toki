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

There are a few general rules:

- **Never run `templ generate`**: The user will use `datapages watch` which automatically runs Templ generation. Running this command will cause irrecoverable race errors that will force the user to restart watch mode.
- **Run `datapages gen`** to check for compilation errors and lint feedback.

### Framework

This application uses the Datapages Go frontend framework, for code requirements, CLI usage, architecture and other instructions see [AGENTS.md](https://github.com/romshark/datapages/blob/main/AGENTS.md).

### Architecture

The editor follows the CQRS (Command Query Responsibility Segregation) architecture as described in [The Tao of Datastar](https://data-star.dev/guide/the_tao_of_datastar). Key principles:

- **Server-side first**: All state and logic lives in Go on the server. The server owns the truth — page state is stored server-side (like `pageTIKsState`, `pageTIKState`) and actions are server POST/PUT/PATCH/DELETE handlers, not client-side signal manipulation.
- **Datastar signals are sparingly used**: Signals should only hold transient UI state (e.g. form input bindings). Never build complex client-side JS expressions to manipulate signals — add a server action instead.
- **JavaScript is a last resort**: Only use JavaScript/TypeScript for things that physically cannot be done on the server (e.g. `matchMedia` dark mode detection, clipboard access, scroll position, etc.). If you're tempted to write JS, consider whether a server POST + SSE morph / SSE signal patch can do it instead.
- **Shared-state actions dispatch events, not patches**: Actions that mutate shared data (e.g. `POSTSet`, `POSTReset`, `POSTApplyChanges`) must use `dispatch` to emit events, not directly patch via SSE. The `OnXXX` event handlers on each active stream pick up the event and patch their respective clients (browser tabs) — this keeps all connected clients in sync. Direct SSE patching from an action handler is only acceptable for client-local view changes that don't affect other clients (e.g. filters, scroll position, UI preferences).
- **Pages showing shared data must handle events**: Implement relevant `OnXXX` handlers on any page that displays data other clients or background processes can modify, otherwise the page goes stale.
- **Tab identity via `instance_id`**: Each browser tab has a unique `instance_id` persisted in `sessionStorage`. It is sent as a signal with every request, allowing the server to maintain per-tab state (filters, scroll position, etc.) and to identify which tab triggered a change (to avoid echo-back during morphs).
- **Page state lifecycle**:
  - `GET` renders the initial page. State is recovered from URL query parameters (e.g. filter type, shown locales/domains) and cookies (e.g. UI preferences like theme, fonts via `ReadUIPrefs`), enabling bookmarkable URLs and persistent preferences without server-side storage.
  - `StreamOpen` initializes server-side state from signals
  - `POST*` methods mutate server state and dispatch events
  - `OnUpdated` patch the page over SSE on events
  - `StreamClose` cleans up. Each page type has its own state struct and maps and that state must not leak but be cleaned up once the SSE stream closes.
- **`reflectsignal` for URL-synced state**: Use the `reflectsignal` struct tag on `GET` query parameters to keep URL query params in sync with Datastar signals. This ensures the URL stays bookmarkable as the user interacts with the page (e.g. changing filters). The server pushes updated signal values via `MarshalAndPatchSignals` after state changes.
- **Avoid `templ.Raw`**: Prefer templ's native constructs over building raw HTML strings whenever possible.
- **Offline-capable**: All assets (CSS, JS) are embedded static files — no CDN dependencies. The app must work offline.
- **Morph safety** — Datastar patches the DOM from SSE responses by morphing elements. For this to work correctly, elements that persist across morphs must have stable `id` attributes so Datastar can match old and new elements. Use `data-ignore-morph` sparingly — only where a morph would destroy in-progress user input (e.g. editable `<toki-editor>` instances). Elements marked with it are invisible to the server's updates, so overuse creates stale UI and complexity.
- **Web components for rich client-side widgets**: Use custom elements (`editor/js/wc/`) for functionality that requires complex client-side state management and rendering (e.g. `<toki-editor>` for ICU message editing with CodeMirror). These are the exception to the server-first rule — they encapsulate self-contained interactive widgets. Web components receive inputs from the server via Datastar signals and attributes (e.g. `data-attr:theme`, `data-bind:value`), keeping the server in control of what the component displays and how it behaves.

### UI Elements

The editor uses [basecoatui](https://basecoatui.com/) (CSS component library based on Tailwind CSS). When building or modifying the editor UI:

- **Prefer basecoat components** (alert, badge, card, button, input, switch, sidebar, etc.) over custom CSS whenever possible.
- **Use modern nested CSS** — group related styles using CSS nesting instead of flat selectors. This keeps styles co-located with their parent context and reduces repetition.
