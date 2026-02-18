# Frontend Redesign — Claude Code Runbook

How to execute the implementation plans using Claude Code without context smearing.

**Core rule: One batch per session. Fresh context beats stale context.**

## CLAUDE.md Addition

Before starting Phase 1, add this section to your project CLAUDE.md. Update it at every phase gate with learnings.

```markdown
## Frontend Redesign (Active)

**Plans:** docs/plans/frontend-redesign/
**Design doc:** docs/plans/2026-02-17-frontend-redesign-design.md
**Current phase:** 1
**Web directory:** web/

### Frontend Build Commands
npm ci                              # Install frontend deps (run from web/)
npm run dev                         # Vite dev server with HMR
npm run build                       # Production build
npx tsx scripts/build-tokens.ts     # Regenerate CSS from tokens.json

### Frontend Testing
npm test                            # Vitest unit tests (from web/)
npx playwright test                 # E2E tests (from web/)
npm run storybook                   # Component stories

### Phase Learnings
(Updated at each phase gate - record what actually happened vs plan)
```

---

## Session Types

### Type 1: Implementation Session (most common)

**When:** Implementing 1–3 deliverables from a phase.

**Prompt template:**

```
Read docs/plans/frontend-redesign/phase-1-foundation.md.

Implement deliverables 1–3 using the executing-plans skill.

Context:
- This is the first batch of Phase 1 (no prior deliverables completed yet)
- The design doc at docs/plans/2026-02-17-frontend-redesign-design.md has
  token definitions, component APIs, and build pipeline details if needed
- Use subagents for any codebase investigation to preserve context

When done with the batch, stop and report what was built + verification output.
```

**Adapt for subsequent batches:**

```
Read docs/plans/frontend-redesign/phase-1-foundation.md.

Implement deliverables 4–6 using the executing-plans skill.

Context:
- Deliverables 1–3 are already complete and committed
- [any specific notes from the previous session, e.g.:]
- We used tailwind.config.ts instead of @theme (Tailwind v4 drift)
- The web/ directory is initialized at web/

When done with the batch, stop and report what was built + verification output.
```

**Key points:**
- Always start with "Read the phase file" — this IS your context
- State which deliverables are already done so Claude doesn't redo work
- Include any drift notes from previous sessions (things that changed from plan)
- "Use subagents for investigation" keeps your main context clean
- Skills invoked: `executing-plans`, `test-driven-development` (when writing testable code)

---

### Type 2: Phase Gate Verification

**When:** All deliverables for a phase are committed. Time to verify exit criteria.

**Prompt template:**

```
Read docs/plans/frontend-redesign/phase-1-foundation.md — specifically the
Exit Gate section.

Verify every exit gate criterion. For each one:
1. Run the verification command/check described in the "How to verify" column
2. Report PASS or FAIL with evidence
3. If FAIL, identify what needs fixing

Use the verification-before-completion skill. Do not claim anything passes
without actually running the verification.

Also check bundle budget (bottom of the phase file).
```

**Key points:**
- This is a *separate session* from the implementation — fresh eyes on fresh context
- The `verification-before-completion` skill forces Claude to actually run checks
- If something fails, fix it in THIS session (small scope) or note it for a dedicated fix session

---

### Type 3: Drift Assessment

**When:** After a phase gate passes, before starting the next phase.

**Prompt template:**

```
I just completed Phase 1 of the frontend redesign.

Read these files:
- docs/plans/frontend-redesign/phase-1-foundation.md (what we planned)
- docs/plans/frontend-redesign/cross-phase-risks.md (drift assessment protocol)
- docs/plans/frontend-redesign/phase-2-design-system.md (next phase)

Now perform the drift assessment from cross-phase-risks.md:
1. What did we actually build vs what we planned? List differences.
2. Which differences are improvements? Keep them, note in the plan.
3. Which differences create downstream problems for Phase 2?
4. What assumptions from the design doc proved wrong?

Then: propose specific edits to phase-2-design-system.md to account for
the drift. Show me the proposed changes before making them.

Drift from Phase 1:
- [You fill these in based on what actually happened, e.g.:]
- Used tailwind.config.ts instead of @theme directive
- auto.ts uses MutationObserver fallback alongside HTMX events
- Props pattern uses data-props for simple cases, script tag for complex
```

**Key points:**
- YOU provide the drift notes — Claude wasn't in those sessions
- This session's job is to update the NEXT phase file, not implement anything
- After this session, update the "Phase Learnings" section in CLAUDE.md

---

### Type 4: Investigation / Spike

**When:** A deliverable has unknowns you want to research before committing to an approach.

**Prompt template:**

```
I need to investigate before implementing Phase 1, deliverable 7 (auto.ts
island loader).

Read docs/plans/frontend-redesign/phase-1-foundation.md deliverable 7 and
the Drift Adaptation table.

Research questions:
1. Which HTMX lifecycle events are actually reliable for mount/unmount?
   Read internal/webadmin/templates/base.html to see current HTMX usage.
2. Does our HTMX version (1.9.10) support htmx:beforeCleanup?
3. What does the MutationObserver fallback look like?

Use subagents for codebase exploration. Do NOT implement anything — just
report findings and recommend an approach.
```

**Key points:**
- Explicit "do NOT implement" — this session is read-only research
- Subagents do the heavy reading, keeping your main context clean for analysis
- Output becomes context you paste into the implementation session prompt

---

### Type 5: Review Session

**When:** A batch is implemented and you want a quality check before moving on.

**Prompt template:**

```
Review the recent frontend changes on this branch. Use the
requesting-code-review skill.

Focus on:
- Does the implementation match the plan in
  docs/plans/frontend-redesign/phase-1-foundation.md deliverables 1-3?
- Are there any security issues (XSS, injection)?
- Does it follow the patterns in CLAUDE.md?
- Is the bundle size within budget?

Use subagents for the review to keep context clean.
```

**Key points:**
- Fresh session, NOT the same one that wrote the code
- Skills invoked: `requesting-code-review`
- Claude reviewing its own code in the same session is less effective — the writer/reviewer split matters

---

## Session Flow Cheat Sheet

```
Phase N:
  ┌─ Session: Investigate unknowns (Type 4, optional)
  │
  ├─ Session: Implement deliverables 1-3 (Type 1)
  ├─ Session: Implement deliverables 4-6 (Type 1)
  ├─ Session: Implement deliverables 7-9 (Type 1)
  ├─ Session: Implement deliverables 10-12 (Type 1)
  │
  ├─ Session: Review implementation (Type 5)
  ├─ Session: Verify exit gate (Type 2)
  │
  ├─ Session: Drift assessment → update Phase N+1 (Type 3)
  └─ Update CLAUDE.md with phase learnings

Phase N+1:
  └─ (repeat)
```

## Rules of Thumb

1. **If you're about to type a second unrelated task in the same session, stop.** `/clear` or start a new session.

2. **If Claude asks "should I also..." for something outside the current deliverable scope, say no.** Scope creep within a session is how context fills up.

3. **If a session has been running >20 minutes, assess whether to continue or commit and restart.** Long sessions degrade. Short focused sessions compound.

4. **Always state what's already done.** Claude can't see git history efficiently. Tell it "deliverables 1-6 are committed" so it doesn't re-investigate.

5. **Drift notes are YOUR job.** You were in the room for the implementation sessions. Carry forward the 2-3 sentences of "what actually happened" as input to the next session.

6. **CLAUDE.md is for stable truths, not session state.** "We use Tailwind v4" goes in CLAUDE.md. "Currently working on deliverable 7" does NOT.

## Prompt Fragments for Common Situations

**When Claude needs to understand existing code:**
```
Use a subagent to read internal/webadmin/templates/base.html and
internal/webadmin/webadmin.go, then summarize how static assets are
currently served and where routes are registered.
```

**When you hit a drift scenario:**
```
This matches the drift scenario in phase-1-foundation.md: "Tailwind v4
@theme doesn't integrate well with Svelte." Follow the adaptation:
fall back to tailwind.config.ts with generated theme extension.
```

**When starting fresh after a bad session:**
```
/clear

The previous session went off track. Let's restart.
Read docs/plans/frontend-redesign/phase-1-foundation.md deliverable 5.
Only implement the /static/ route — nothing else.
```

**When a deliverable needs creative decisions:**
```
Read docs/plans/frontend-redesign/phase-1-foundation.md deliverable 7.
Use the brainstorming skill to explore approaches for the island
auto-loader before implementing. I want to review the approach first.
```
