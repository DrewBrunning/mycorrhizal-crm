---
name: documentation-maintenance
description: Use when a behavior, command, configuration, schema, or generated artifact change has documentation impact.
license: MIT
---

# Documentation maintenance

Outcome: the prose that describes the change is updated in the same PR, and no
generated document was hand-edited. Read `docs/AGENTS.md` first.

Open [generated-docs-map](references/generated-docs-map.md) before editing a
document that looks generated: it maps each generated doc to its generator and
drift test.

## Steps

1. Find every page that makes the claim you changed: the platform
   `AGENTS.md`, the operator page (`docs/supported-versions.md`,
   `docs/integration-ownership.md`, `docs/configuration-reference.md`), and the
   canonical policy page. Update the canonical page and the pages that link to
   it, not a new copy.
2. If the change moves or renames a file/heading/test, update every link and
   citation in the same PR. `docscheck` resolves docs links/anchors/repo paths;
   `citecheck` resolves security-doc `path#anchor` and test-name citations.
3. If the doc is generated, regenerate it with the map's command, commit the
   diff, and run the named drift test. Never hand-edit a generated file.
4. If a new persistent copy of user data or a trust boundary appears, update
   `docs/security/data-retention-lifecycle.md` and/or
   `docs/security/threat-model.md` in the same change.
5. Keep the operator-facing half in step with the engineering matrix
   (`docs/supported-versions.md` ↔ `docs/development/supported-runtime-matrix.md`,
   `docs/integration-ownership.md` ↔ the integration registry).
6. Run the structural gates; docs-only changes are verified by them, not by a
   new unit test.

## DoD

```bash
cd backend && go run ./cmd/docscheck
cd backend && go run ./cmd/citecheck
```

Report any check you could not run; never imply it passed.

## Stop and ask

- A generated doc's diff would be hand-written rather than regenerated.
- A citation points at a line or a deletion instead of a declaration/step.
- An operator-visible change has no `## Upgrade notes` (or `no-changelog:`) in
  the PR body.
- The docs would need to duplicate a canonical policy page.

Canonical detail: `docs/AGENTS.md`, `docs/changelog-policy.md`,
`docs/development/context-engineering.md`.
