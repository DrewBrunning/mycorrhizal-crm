# Continuous SBOM/VEX monitoring (Dependency-Track) — evaluation

**Decision: not adopted for `v1.5.1`.** Grype remains the vulnerability gate and the signed
SPDX/CycloneDX SBOMs remain the published artifact. The spike found a small amount of net-new signal,
but all of it was in dev-only dependencies, it arrived with enough false positives to need VEX
triage, and the dashboard had no consumer. Recorded from issue
[#1882](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1882).

## What was evaluated

A self-hosted OWASP Dependency-Track stack in local Docker Compose only — no SaaS upload, no
application traffic, no new runtime dependency:

| Component | Version |
|---|---|
| Dependency-Track API server / frontend | `4.14.5` |
| PostgreSQL | `17-alpine` |
| Syft (SBOM producer, same version as `syft-sbom.yml`) | `v1.51.0` |
| Grype (existing gate, same version as `grype.yml`) | `v0.117.0`, DB built `2026-10-23T06:30:15Z` |
| Source | `main` at `3655440a6` |

The CycloneDX 1.7 SBOM was generated with Syft over the source tree exactly as `syft-sbom.yml` does
(no dependency install), then uploaded to a locally created project. Dependency-Track was allowed to
mirror OSV `Maven`, `Go`, and `npm` (the three ecosystems with non-trivial component counts); the
NIST mirror was stopped after the 2026/2025 feeds so the comparison could finish. Grype scanned the
same tree with the committed `.grype.yml`.

## Results

**SBOM:** 5,389 components (5,303 libraries, 86 files), 7.0 MB. Package types: 3,892 Maven, 936 npm,
226 GitHub, 94 Go, 66 Cargo, 40 RubyGems, 22 PyPI, plus unpinned file/library entries.

| Tool | Findings | At/above the existing gate (`high`+) |
|---|---:|---:|
| Grype (no CPE override) | 1 | 0 (the one finding is `UNASSIGNED`) |
| Dependency-Track | 18 | 15 |

The single Grype finding is `GO-2026-5932` (`golang.org/x/crypto@v0.57.0`), which Dependency-Track
also reported. Every other Dependency-Track finding was net-new to this comparison:

| Finding group | Count | Verdict |
|---|---:|---|
| `GO-2026-5932` (`golang.org/x/crypto`) | 1 | duplicate of Grype |
| `CVE-2026-109383`, `109829`, `109841`, `109869`, `109924`, `109932`, `87018`, `87029` (`undici` 8.12.1) | 8 | real advisories, dev-only (`jsdom`) |
| `GHSA-mrhx-c7xc-m3w5` (`zod` 4.4.3) | 1 | real advisory, dev-only (`mutation-server-protocol`/ESLint) |
| `GHSA-4qv8-2p9j-2685`, `GHSA-f72v-6w45-q472`, `GHSA-xjj6-f33q-929c` (`undici`) | 3 | alias duplicates of three NVD `undici` rows above |
| `CVE-2026-11822`, `CVE-2026-11824`, `CVE-2025-6965`, `CVE-2025-70873` (`androidx.sqlite:sqlite` 2.7.0) | 4 | false positive: CPE `sqlite:sqlite:2.7.0` was generated for the AndroidX Java wrapper, not the SQLite C engine |
| `CVE-2026-0994` (`google.golang.org/protobuf` 1.36.11) | 1 | false positive: the CVE is in Python `json_format`; this is the Go runtime |

After collapsing the three NVD/GHSA aliases, the net-new signal is **9 distinct advisories** — 8
`undici` and 1 `zod` — and **5 false positives**. None of the real net-new advisories is in the
shipped frontend bundle: `undici` is pulled in by `jsdom` (a `devDependency` used by Vitest), and
`zod` is pulled in by dev tooling (`mutation-server-protocol` / `eslint-plugin-react-hooks`). The
source-tree SBOM includes dev dependencies; a release-image SBOM would not contain them.

### VEX effort

Five false-positive decisions were recorded to suppress the `sqlite` and Go-protobuf findings. Each
decision is one `PUT /api/v1/analysis` with the component UUID, vulnerability UUID, state,
justification, response, and a free-text detail — 5 decisions took 0.24 s through the API. The
exported decision set is a 32 KB CycloneDX 1.5 VEX document (all 18 findings, most with no state).

That is mechanically cheap, but the decisions live in Dependency-Track's database. Keeping them
reviewable in-repo (the project's existing convention, `.grype.yml`'s ignore-list-with-justification)
would require committing a VEX file and uploading it on every run, making a second artifact that can
drift from the DB. The alias sync is off by default, which is why three NVD/GHSA pairs appeared as
separate rows until collapsed by hand.

### Operational cost

- Dependency-Track's documented container minimum is **2 GB RAM / 2 CPU** for the API server and
  **8 GB / 4 CPU** recommended, plus the frontend container and PostgreSQL. The spike's idle
  API server used ~900 MB with a 4 GB heap; the 18-finding analysis briefly used ~40% CPU.
- A first NIST mirror is a long download: in the spike it was still processing annual feeds after
  five minutes at roughly two minutes per feed; a full 2002–2026 pass is on the order of an hour.
  OSV was a better fit for the language ecosystems: the full `Maven` + `Go` + `npm` mirror took
  **7 minutes 04 seconds** (230,735 npm advisories alone).
- The service has its own monthly mirror cadence, version upgrades, database backup, and a scoped
  API key if CI uploads to it. None of that is a merge gate; a dashboard is not a gate.

## Why this is not adopted

1. **No consumer.** This project's response path is a required status check plus the GitHub Security
   tab; there is no on-call or release manager whose job is to open a separate dashboard. Adding one
   would be a new process with no owner, not a CI change. The issue's own decision comment said to
   reject if the dashboard would not actually be consulted.
2. **The net-new signal is dev-only.** The 9 real distinct advisories are transitive dev
   dependencies. Grype already scans the source tree nightly (including dev dependencies) and the
   release image is separately covered by Trivy; the incremental value is continuous re-matching at
   most 24 hours ahead of the nightly run, not new runtime coverage.
3. **The false positives cost more than the signal.** The five CPE-derived false positives are
   exactly the triage that a dashboard is supposed to make cheap, but they still require a VEX
   decision per finding and a second decision store to keep reviewable.
4. **The gate stays Grype regardless.** Dependency-Track cannot replace the nightly gate, so it would
   be an additional service plus an upload secret for a second opinion the nightly gate already
   produces.

## Revisit only if

- a second maintainer/on-call owns a weekly dashboard review, or
- the SBOM uploaded is the **release-image** SBOM (runtime only) rather than the source-tree SBOM,
  with alias sync enabled and VEX committed in-repo, or
- a runtime advisory is found that Grype/Trivy cannot see but Dependency-Track does.

Until one of those is true, this is a recorded rejection rather than an untracked gap. The existing
Grype gate, the signed SBOMs, the GitHub Security tab, and `.grype.yml`'s justified ignores remain the
supply-chain monitoring surface.

## Spike notes

The comparison was run entirely on the local machine:

- `anchore/syft:v1.51.0 dir:/src -o cyclonedx-json`
- `anchore/grype:v0.117.0 dir:/src -c .grype.yml -o json`
- `dependencytrack/apiserver:4.14.5` + `dependencytrack/frontend:4.14.5` + `postgres:17-alpine`;
  OSV ecosystems `Maven;Go;npm`; `nvd.enabled=false` after the 2026/2025 feeds; OSS Index disabled.
- Findings were read from `GET /api/v1/finding/project/{uuid}?suppressed=true`; decisions from
  `PUT /api/v1/analysis`; VEX from `GET /api/v1/vex/cyclonedx/project/{uuid}`.

No data or artifact left the machine, and no repository file is consumed by the spike.
