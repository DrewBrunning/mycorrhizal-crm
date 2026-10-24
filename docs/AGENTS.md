# Documentation and security docs

Read the root `AGENTS.md` first. Docs are code here: they are gated by `docscheck`, `citecheck`,
`depexceptions`, `deprecations`, `releasegatecheck`, `governancecheck`, `pragmacheck` and
`nightlyalertcheck`, all of which CI runs unconditionally because `.github/filters.yaml` maps
`docs/**` to nothing. A documentation-only change is verified by these gates rather than by the
test-first workflow; a change with runtime behaviour is not documentation-only.

## Structural gates

- `cd backend && go run ./cmd/docscheck` (requires a repo checkout and the Go toolchain) verifies
  every internal link/anchor and repo-file reference in `docs/`, that operator command blocks do not
  assume a toolchain their audience lacks, that the operator versions page and
  `docs/development/supported-runtime-matrix.md` state the same floors, that every integration in
  `backend/integrations.Registry()` has an ownership section in `docs/integration-ownership.md`,
  and that every migrated table appears in `docs/security/pii-inventory.md` (or the reasoned
  exclusion list). Moving or renaming a file/heading any doc links to means updating the link in the
  same PR — docscheck is the tripwire.
- `cd backend && go run ./cmd/citecheck` proves every `path#anchor` and test-name citation in
  `docs/security/asvs-l2.md`, `masvs-l1.md` and `threat-model.md` still resolves, and that no
  `satisfied` row cites nothing. **Cite a thing, never a line:** `path#AuthMiddleware`,
  `path#Type.Method`, `path#Run Trivy vulnerability scanner`, a Kotlin/TS declaration, or
  `path#"literal text"` for config/SQL. A `path:line` citation is a gate failure. One anchor names
  one thing; two declarations are two citations. citecheck proves the anchor exists, not that the
  row still describes what it does — a verification pass re-reads that.

## Generated docs

Generated docs are **never hand-edited**; regenerate and commit the diff. This includes
`docs/data-01-field-compatibility-matrix.md` (`gencompatmatrix`),
`docs/int-01-integration-classification-matrix.md` (`genintegrationmatrix`), the schema dumps under
`backend/database/testdata/schemas/` (`genschema`), `docs/development/perf-benchmarks.md` /
`data-movement-benchmarks.md` (`perfbench`), and the config reference. The drift test named by each
generator fails until it is regenerated; the diff is the review.

## Security, ASVS/MASVS, retention

The security checklist is `docs/security/asvs-l2.md` (OWASP ASVS 4.0.3 L2 plus the API Security
Top 10) and `docs/security/masvs-l1.md` for Android. A security-sensitive PR **updates the row(s) it
touches** — status and citation — in the same commit; if the change cannot point at a row, it does
not know what it is changing. N/A rows are written-down decisions (single process, self-hosted) that
flip to real statuses if the architecture changes.

The project's self-assessed claim is **ASVS L2 with 23 documented exceptions** and **MASVS-L1 with 1** — see the verification report §6; this is not a third-party audit. `docs/security/asvs-l2-verification-report.md`
records *how* the checklists were verified and *when*: add a changelog row there for every
re-verification rather than silently editing statuses. Do the §8 re-verification pass when closing a
milestone gate, not at release-dispatch time — `release.yml` refuses to tag if §10 has no row since
the previous release tag (issue #608). If commits land between gate-close and dispatch, redo the
diff-review for just those.

Every data type's retention/deletion lifecycle is `docs/security/data-retention-lifecycle.md`. A new
persistent copy of user data (external sync target, client-side cache, export format) gets an entry
there in the same PR. `docs/security/pii-inventory.md` is the other half and is checked against the
live schema. The threat model (`docs/security/threat-model.md`) is updated when a design change adds
a trust boundary.

## Policy pointers

- Coverage gate: `docs/development/coverage.md`; write the test first, and use
  `// # pragma: no cover — <reason>` only for structurally unhittable lines (`pragmacheck` enforces
  the reason).
- Operator docs: `docs/supported-versions.md` and `docs/integration-ownership.md` are the
  operator-facing halves of the runtime and integration matrices; keep them in step with the
  engineering pages.
- Breaking-change and deprecation policy: `docs/breaking-change-policy.md`,
  `docs/deprecation-policy.md`, `docs/versioning-policy.md`, `docs/client-compatibility-policy.md`.
- Changelog: an operator-visible change needs an `## Upgrade notes` block (or `no-changelog: <reason>`)
  in the PR body — `docs/changelog-policy.md`.
- Dependency upgrades: `docs/dependency-upgrade-policy.md`; a time-bounded exception goes in
  `docs/security/dependency-exceptions.ignore` and `cmd/depexceptions` fails once it expires.
