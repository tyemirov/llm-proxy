# AGENTS.FRONTEND.md

## Scope

Guidelines for building the browser front end with semantic web components, Alpine.js, and vanilla CSS. Follow AGENTS.md for shared workflow and POLICY.md for validation rules.

## Front-End Philosophy

### Semantic Web Components

- Author interfaces with semantic HTML or lightweight custom elements that mirror the domain (e.g., `<note-card>`, `<history-panel>`). Avoid anonymous `<div>` stacks.
- Keep markup declarative: structure communicates intent and lifecycle; avoid imperative DOM creation when semantics suffice.
- Encapsulate visual states with attributes or data-* flags that Alpine can toggle, keeping templates readable.

### Alpine-Driven Behavior

- Alpine factories (`function NoteCard() { return { ... } }`) own component behavior. Bind them via `x-data` on the semantic wrapper element.
- Co-locate Alpine directives (`x-bind`, `x-on`, `x-show`, etc.) with the markup they affect. The template should read like a specification of behavior.
- Business logic belongs inside Alpine stores/factories or pure helpers in `js/core` and `js/utils`. Templates only orchestrate declared behaviors.
- Prefer Alpine events over manual DOM APIs. Dispatch intent-specific events (`$dispatch('note:saved', payload)`) from child components and listen inside parent scope.

### Declarative Flow

- Local state lives inside each component; introduce shared stores only when multiple semantic components must coordinate.
- Treat events as the communication boundary between components. Avoid `.window` listeners unless no other scope fits.
- Eliminate dead code, unused imports, or duplicate logic. Extract shared behaviors into helpers or Alpine factories reused via composition.

## Component Authoring

### Naming & Identifiers

- No single-letter or non-descriptive names.
- `camelCase` for variables/functions, `PascalCase` for Alpine factories or classes, `SCREAMING_SNAKE_CASE` for constants.
- Event handler functions describe intent (`handleSpinButtonClick`, not `onClick`).

### Template Semantics

- Start each component with a semantic wrapper element or custom element name.
- Use attributes (e.g., `data-state="loading"`) or CSS classes driven by Alpine to reflect business state; do not hardcode styling logic inside JavaScript.
- Keep markup tidy: avoid deeply nested anonymous wrappers, and prefer slot-like child elements for content sections.

### Strings & Enums

- All user-facing strings belong in `js/constants.js`.
- Freeze enum-like objects with `Object.freeze` or use symbols.
- Map keys must be named constants, never ad-hoc strings.

### State & Events

- `x-data` owns localized state. Do not mutate imports or global objects.
- Shared state goes through `Alpine.store` only when multiple components truly require it.
- Use `$dispatch`/`$listen` for cross-component communication and keep events scoped to component boundaries where possible.
- Notifications/modals must respond to explicit events; they never self-open.

## Structure & Dependencies

### Code Organization

- ES modules everywhere (`type="module"`). Enable strict mode implicitly through modules.
- Pure transforms in helpers; Alpine factories for stateful UI behavior.
- Directory layout:

  ```
  /assets/{css,img,audio}
  /data/*.json
  /js/
    constants.js
    types.d.js
    utils/
    core/
    ui/
    app.js
  index.html
  ```

- DOM logic lives in `js/ui/`; domain logic in `js/core/`; reusable helpers in `js/utils/`.
- Do not mutate imported bindings or function parameters.

### Dependencies & Versions

- CDN dependencies only; no bundlers.
- Alpine.js: `3.13.5` from `https://cdn.jsdelivr.net/npm/alpinejs@3.13.5/dist/module.esm.js`
- Google Identity Services: `https://accounts.google.com/gsi/client`
- Loopaware widget: `https://loopaware.mprlab.com/widget.js`

## Testing & Quality

- Testing follows an **inverted test pyramid**: Playwright-driven black-box integration/end-to-end tests are the primary mechanism and should drive coverage toward (approximately) 100%, with CI enforcing an agreed threshold.
- Playwright is the standard browser automation framework; Puppeteer and other harnesses are not allowed.
- For user-visible behavior, tests MUST exercise the real deployed code path through the browser (no bypassing Alpine wiring or DOM behavior).
- Integration tests only; unit tests are prohibited.
- Perform semantic testing of component visibility, accessibility, and behavior through Playwright scenarios.
- `make test` runs the Playwright harness headless (it may delegate to package scripts).
- When you have many scenarios, table-drive Playwright cases (data sets) to cover permutations.
- Black-box tests only: assert observable behavior and events, not internal DOM wiring.

## Documentation & Refactors

- Every public function and Alpine factory includes JSDoc and `// @ts-check` at the top of each module.
- `types.d.js` stores shared typedefs (e.g., `Note`, `NoteClassification`).
- Each domain module receives a `doc.md` or `README.md` explaining its responsibilities.
- Plan refactors with bullet plans and split files larger than ~300–400 lines.
- `app.js` remains the composition root: registers stores, components, and bridges events.

## Error Handling & Logging

- Throw `Error` objects—not strings—and catch errors at user entry points (button actions, init routines).
- Use `js/utils/logging.js` for logging; no stray `console.log`.

## Performance & UX

- Use Alpine `.debounce` modifiers for inputs.
- Batch DOM writes with `requestAnimationFrame`.
- Lazy-initialize heavy components (editors, charts) on first intersection/interaction.
- Cache selectors, avoid forced reflows, and keep animations asynchronous (no blocking waits).

## Linting & Formatting

- Run lint/format via the repo `Makefile` targets (prefer `make lint` / `make fmt` / `make ci`). If ESLint runs in Docker, the Makefile target should encapsulate that so contributors do not need ad-hoc commands.
- Prettier runs only when explicitly requested; never on save.
- Enforced rules include `no-unused-vars`, `no-implicit-globals`, `no-var`, `prefer-const`, `eqeqeq`, and `no-magic-numbers` (allowed: -1, 0, 1, 100, 360).

## Data > Logic

- Validate static JSON catalogs during boot.
- Treat validated data as trustworthy; fail fast if validation fails.

## Security & Boundaries

- No `eval` or inline `onclick`.
- CSP is optional but recommended; only Google Analytics may remain inline per policy.
- All external network calls go through `js/core/backendClient.js` or `js/core/classifier.js`. UI layers never call `fetch` directly.
