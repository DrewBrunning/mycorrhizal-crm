---
name: dependency-update-review
description: Use when upgrading, downgrading, adding, or removing a dependency, base image, Gradle entry, or GitHub Action.
license: MIT
---

# Dependency update review

Outcome: the dependency is updated through its own package manager, the lockfile
is committed, the notices/SBOM artifacts are current, and the change is reviewed
at the tier the policy assigns. Read `docs/dependency-upgrade-policy.md` first;
it is the canonical tiering policy, not restated here.

Open [per-ecosystem](references/per-ecosystem.md) for the exact command,
lockfile, scanner-ignore file, and generated artifact in the ecosystem you are
touching.

## Steps

1. Classify the change: security fix, patch/minor batch, or major/add-new. Major
   bumps and new direct dependencies are deliberate, single-purpose PRs.
2. Update through the ecosystem's manager: `go get`/`go mod tidy`, Yarn with the
   committed lockfile, the Gradle version catalog + lockfile, a pinned Docker
   digest, or a SHA-pinned Action. Never vendor source or fetch at build time.
3. Run the affected build/tests plus `go run ./cmd/depexceptions` so an expired
   or unnecessary advisory exception fails before review.
4. Regenerate the third-party notices (`make gen-notices`) and commit the diff;
   a dependency bump that does not regenerate fails the notices drift gate.
5. Review the scanners the ecosystems feed: Trivy/grype ignores, the
   `.mobsf`/`zap`/`schemathesis` lists, and the vulnerability advisories. An
   ignore entry needs a reason and, for a time-bounded exception,
   `docs/security/dependency-exceptions.ignore`.
6. If a minimum runtime moved, update `docs/development/supported-runtime-matrix.md`
   and the operator-facing `docs/supported-versions.md` in the same PR.

## DoD

```bash
cd backend && go run ./cmd/depexceptions
cd backend && make gen-notices
```

Report any check you could not run; never imply it passed.

## Stop and ask

- A major bump or new dependency has no release-notes rationale.
- The license is copyleft or unknown.
- A Docker image or Action would become unpinned.
- A scanner ignore would hide an unfixed critical/high without an exception.
- A supported-runtime floor would move without the compatibility-matrix update.

Canonical detail: `docs/dependency-upgrade-policy.md`, `docs/development/supported-runtime-matrix.md`,
`docs/security/dependency-exceptions.ignore`.
