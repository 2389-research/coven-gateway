# Phase 5: Login & Auth

**Weeks:** 11
**Depends on:** [Phase 4 gate](phase-4-dashboard-admin.md#exit-gate)
**Goal:** Unify the login/auth experience with the new design system. WebAuthn flows migrated to Svelte components.

## Deliverables

| # | Task | Detail |
|---|------|--------|
| 1 | Login page redesign | New login template using design tokens. Username/password form using `TextField`, `Button` components. |
| 2 | WebAuthn Svelte components | Migrate passkey registration + login from inline JS to Svelte components. Proper error handling, loading states, browser support detection. |
| 3 | Setup flow | Migrate `setup.html` and `setup_complete.html` to new design system. |
| 4 | Invite flow | Migrate `invite.html` to new design system. |
| 5 | CSP headers | Add Content-Security-Policy headers now that CDN scripts are removed. Strict policy: no inline scripts (all Svelte-compiled), no external sources. |
| 6 | Dark theme (if not shipped earlier) | With all pages now on design tokens, enable dark theme via `data-theme="dark"`. Server sets from cookie/system preference. |

## Exit Gate

| Criterion | How to verify |
|-----------|---------------|
| WebAuthn E2E green | Playwright with CDP Virtual Authenticator: register passkey → login with passkey → full flow works |
| CSP zero violations | Load every page — zero CSP violation reports in console |
| CSRF validated | All POST routes still validate CSRF tokens correctly |
| Login flow complete | Username/password login, passkey login, setup flow, invite flow — all functional |
| Design consistency | All pages (login, chat, admin) share the same visual language from design tokens |
| CDN-free | Zero external CDN dependencies. `base.html` loads nothing from unpkg, jsdelivr, googleapis, or tailwindcss.com |

## Drift Adaptation

| If this happens... | Then adjust... |
|--------------------|---------------|
| WebAuthn JS is too tightly coupled to inline HTML | Keep WebAuthn as vanilla JS loaded alongside the login island, not inside a Svelte component. The important thing is design consistency, not framework purity. |
| Dark theme has too many contrast issues | Ship without dark theme. Add it as a separate Phase 5b after manual audit of all semantic token mappings against WCAG 2.1 AA. |
| CSP breaks third-party integrations added later | Use `nonce`-based CSP instead of hash-based. Generate per-request nonces in Go, pass to template. |

## Bundle Budget

| Asset | Limit (gzipped) |
|-------|-----------------|
| JS | 150KB (unchanged from Phase 4) |
| CSS | 30KB (unchanged from Phase 4) |
