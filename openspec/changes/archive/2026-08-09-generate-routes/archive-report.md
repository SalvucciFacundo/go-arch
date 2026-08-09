# Archive Report: generate-routes

**Status**: ARCHIVED
**Archived**: 2026-08-09
**Verify verdict**: PASS 28/28 (base 16/16 compliant + 2 partial + cli delta; after the pre-change routes.go CRITICAL fix, commit 2865db0)
**Delivery mode**: Ordinary policy — receipt-driven development (review gate) disabled at clone scope (user decision after escalating upstream #2743). No receipt required; nothing silently approved.

## Final-State Facts

- **Tasks**: 18/18 complete (3 slices + the pre-change routes.go CRITICAL fix).
- **Slices**: PR1 routes.tmpl + manifest Routes (1418175) · PR2 manifestDir fix + registry renderer + CRUD/handler wiring (9a8f2e3) · PR3 upgrade/main.tmpl/CLI/MCP wiring (2acda01, 0ba43b3) · CRITICAL fix (2865db0).
- **Verify PASS 28/28**: 174+ tests green, vet/gofmt/golangci-lint clean, live: fresh web `new` compiles immediately; `generate crud User` registers 5 routes idempotently; `generate handler --route` adds a route; non-web hint only; main.go byte-identical; **upgrade creates routes.go for pre-change web projects (CRITICAL fixed) + legacy fallback**; nested-dir fix; MCP route param.
- **Non-blocking WARNING**: "manual registration hint" clause for `generate handler X` without `--route` not implemented (registry-untouched verified; hint clause unimplemented).

## Artifacts

- exploration.md ✅ · proposal.md ✅ · specs/cli/spec.md (delta) ✅ · design.md ✅ (corrected 7 defects) · tasks.md ✅ (18/18, marked ARCHIVED) · verify-report.md ✅ · archive-report.md ✅

## Spec Sync

- Delta `cli` (3 ADDED + 1 MODIFIED requirements) appended to `openspec/specs/cli/spec.md` (generate handler --route flag, CRUD default-on, help, oops codes extension).
- Capability spec `openspec/specs/generate-routes/spec.md` verified in place (no copy).

## Next Steps (Delivery)

- **4 chained PRs** (feature-branch-chain): PR1 feat/generate-routes-1 → tracker; PR2 -2 → -1; PR3 -3 → -2; PR4 -4 → -3; then tracker → main. Rollback = independent per-PR revert.
- Include verify verdict + WARNING in PR bodies.

## Rollback Note

Independent reverts per slice (manifest/routes.tmpl, scaffold core, wiring, fix). Existing projects roll back via `upgrade --yes` to prior main.tmpl. Archived folder is an immutable audit trail.

## Delivery-Mode Note

Receipt-driven development (review gate) disabled at clone scope per user decision after escalating upstream #2743. Archive under ordinary delivery policy — no receipt required, nothing silently approved.
