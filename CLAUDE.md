# Editor UI

The editor (`editor/`) uses [basecoat](https://basecoatui.com/) (CSS component library based on Tailwind CSS). When building or modifying the editor UI:

- **Prefer basecoat components** (alert, badge, card, button, input, switch, sidebar, etc.) over custom CSS whenever possible.
- Basecoat is loaded via embedded static files (`editor/app/static/basecoat.min.css` and `basecoat.min.js`), not CDN — the app must work offline.
- Use basecoat's class naming: `badge-destructive`, `badge-secondary`, `badge-outline`, `alert`, `alert-destructive`, `btn`, `btn-primary`, `btn-ghost`, `input`, `card`, `sidebar`, etc.
- Alerts use `<section>` for content: `<div class="alert"><section>text</section></div>`.
- Badge variants are standalone classes (not combined with base `badge`): use `badge-destructive` not `badge badge-destructive`.
- Dark mode uses the `.dark` class on `<html>`, managed via `localStorage` and a `MutationObserver`.
