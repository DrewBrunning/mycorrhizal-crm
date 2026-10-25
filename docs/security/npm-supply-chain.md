# npm supply-chain behaviour scanning — evaluation

**Decision: adopted GuardDog, self-hosted, as an advisory scan** over the tree installed from
`frontend/yarn.lock`, plus a direct install-time lifecycle-script inventory. Recorded from issue
[#1878](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1878); the baseline was taken after
[#1907](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1907) removed the unused frontend
packages, as the issue required.

This is a **fourth** dependency signal, not a replacement for the CVE and license layers:

| Layer | Question it answers | Where |
|---|---|---|
| Dependabot + Dependency Review | is a newly added package known-vulnerable, or a dependency-confusion/typosquat pattern? | `dependency-review.yml` |
| govulncheck / Trivy / Grype | is a known CVE reachable or present? | `unit-tests.yml`, `docker-publish.yml`, `grype.yml` |
| Trivy license scan | may we ship this license? | `license-compliance.yml` |
| **GuardDog + lifecycle inventory** | **does the installed code behave like malware, or run an install hook?** | `npm-supply-chain.yml` |

The last question is the one none of the others answer. A poisoned release, a compromised
maintainer account, or a hostile `preinstall` hook need not have a CVE at all, and a license scan
will happily approve it.

## What was evaluated

Three self-hosted-first candidates from the issue, in the order its decision comment set:

| Tool | yarn 1 lockfile | Repo access | Signal | Verdict |
|---|---|---|---|---|
| **GuardDog** 3.2.0 (Datadog, Apache-2.0) | No project/lockfile parser — `npm verify` reads `package.json` only | None; CLI runs in CI | Semgrep/YARA behaviour rules + npm metadata heuristics | **Adopted**, with a wrapper that scans the installed tree |
| `lockfile-lint` 5.1.0 | Yes | None | Registry URL / HTTPS / integrity-hash policy only | Not adopted; a partial control that does not detect malware |
| Socket.dev CLI 1.10.1 / GitHub App | Yes | App install or CLI token; metadata leaves the runner | Behavioural analysis, typosquat/malware | Not adopted; SaaS access is unnecessary when the self-hosted option meets the need |

GuardDog's own `npm verify` is not usable as-is here: it parses `package.json` ranges and resolves
them against the **live registry**, so findings drift when upstream publishes and do not describe the
locked tree. It also cannot see transitive install-time code. `frontend/scripts/guarddog-scan.mjs`
therefore runs `yarn install --frozen-lockfile --ignore-scripts` and then GuardDog's local package
scanner over every installed package directory (including nested copies). That is exact lockfile
fidelity, no registry dependence, and no source upload.

`lockfile-lint` was rejected as the malware layer because it checks only where a package was fetched
from and that its hash matches — useful for registry-URL drift, but it cannot distinguish a malicious
tarball that is correctly served by npmjs.org. It is not run in addition: the lockfile already fixes
the registry and integrity hashes, and the pre-commit secret scan does not depend on it.

Socket.dev's signal is the closest to GuardDog's, and it does understand `yarn.lock`. It was not
chosen because it requires granting a third party repository access (App) or a CLI token, and the
self-hosted GuardDog scan already produces the behaviour signal the issue asked for. No Socket token
or App exists in this repository, so adding a job for it would be a new credential and a new
external data flow for no additional coverage this tree needs.

## Results on this tree

Baseline: `main` at `4a447a252` (post-#1907), `frontend/yarn.lock`, GuardDog 3.2.0 with Semgrep
1.174.0, Node 22, `yarn install --frozen-lockfile --ignore-scripts`.

| Measurement | Result |
|---|---|
| Installed packages scanned | 932 (including nested duplicate versions) |
| **Install-time lifecycle scripts** (`preinstall`/`install`/`postinstall`) | **0** |
| GuardDog findings | 663 in 932 packages |
| `capability-*` findings (API reachability, not intent) | 574 |
| `threat-*` findings | 89 |
| Threat findings after per-package review | **all false positives** |

A re-run on 2026-10-25 at `main` `cf135ce64` (the v1.5.1 branch after #1986's `undici`/`zod` bump)
scanned **931 packages / 662 findings**, with **0 new findings, 0 install-time lifecycle scripts and no
stale-entry warnings**: every finding still matched `frontend/guarddog.ignore`. The one-package/
one-finding drop from the baseline is the dependency-tree change since `4a447a252`; no review line
needed changing. Issue #1884 records this as the post-fix clean run.

The 89 threat findings come from 24 well-known build/test packages. They were reviewed against their
source and fall into four classes:

- **Metadata URL regex too broad**: `@types/react`, `axe-core`, `tree-kill`, `tinycolor2` and friends
  put `http://` repository/homepage/funding URLs in `package.json`; the rule looks for HTTP
  dependencies, not metadata.
- **Bundled/minified distributions**: Babel, Terser, Playwright and `d3-scale-chromatic` embed
  unicode identifier tables, compression fixtures, and their own minified output; the
  obfuscation/homoglyph rules read the density of those strings as packing.
- **Tooling doing its job**: Playwright downloads and spawns browsers, Vite probes WSL/container
  networking and reads project env files, Knip enumerates `.env` files to report unused keys.
- **Documentation URLs**: Workbox's error messages use its own `https://bit.ly/wb-precache`
  shortlink.

Each accepted finding is one line in `frontend/guarddog.ignore` with its package-specific reason.
`capability-*` findings are accepted as a class (one line, with the rationale) because they describe
API reachability — every application and every build tool uses the network, spawns processes, and
reads files. The malware signal is in the `threat-*` rules, which are reviewed individually.

Impact: **43 seconds** for all 932 packages at `--jobs 8` on a 4-vCPU runner-class machine, plus the
cached `yarn install`. This is one advisory CI job.

## Install-time lifecycle scripts

GuardDog's npm `threat-npm-preinstall-script` YARA rule matches `package.json`, but local scans use a
default YARA extension exclusion list that contains `json`, so the rule cannot fire on the installed
tree. The wrapper therefore inventories `preinstall`, `install`, and `postinstall` scripts directly
when it reads each package's manifest and emits a `npm-lifecycle-script` finding.

The current tree has none. That is not a reason to skip the check: a new dependency with an install
hook is exactly the event this layer exists to surface, and the wrapper is the one place that sees
the manifest after a lockfile-faithful install. A benign new hook is accepted by adding it to
`frontend/guarddog.ignore` with the reason.

## Coverage limits

- **Behaviour, not a CVE database.** It catches known malware *shapes* (download-and-execute,
  environment exfiltration, silent spawning, obfuscation) and new install hooks. It does not prove a
  package is safe, and it will not detect a targeted, clean-looking backdoor.
- **Dev dependencies are in scope on purpose.** Build-time tooling runs with repository credentials
  on CI and with the developer's shell locally; "does not ship in the image" is not "not attack
  surface".
- **The installed tree is platform-specific.** `yarn install` on Linux omits optional packages for
  other OSes, the same way the existing license and CVE scans resolve the Linux tree.
- **Pinned scanner.** GuardDog 3.2.0 and Semgrep 1.174.0 are pinned; a rule/engine bump is a
  deliberate PR that re-baselines the ignore list, not a moving `latest`.
- **Advisory tier.** A new finding is visible in the Security tab and turns the job red, but it does
  not block a merge or release. Promotion to a required check needs the normal governance change
  (`release-gates.json`, the `main-protection` ruleset, and the release-gates docs) once the
  baseline has proven stable.

## Operating it

The scanner needs the frontend tree installed without executing hooks, then the pinned GuardDog:

```bash
cd frontend && yarn install --frozen-lockfile --ignore-scripts
uv tool install --force --python 3.12 --with 'semgrep==1.174.0' 'guarddog==3.2.0'
node frontend/scripts/guarddog-scan.mjs --output guarddog.sarif --jobs 8
```

A new finding prints `NEW <rule> | <package>@<version> | <location>`, is written to the SARIF, and
exits 1. Review it against the package source:

- **True positive** — remove/quarantine the dependency; do not add an ignore line.
- **False positive** — add `rule | package | reason` to `frontend/guarddog.ignore`. Use a rule
  prefix ending in `*` only when every current and future match of that prefix is covered by the
  same reason, and say why in the reason. Stale entries are reported as warnings, not failures, so
  the list stays honest without making a version bump noisy.

To re-baseline after a deliberate dependency change, run the commands above and review the diff of
`frontend/guarddog.ignore` in the same PR.

## Hand verification

On a scratch branch (not committed), a benign local package with `"preinstall": "node seed.js"` was
added as a dev dependency, installed with `--ignore-scripts`, and the scan re-run:

```
GuardDog scanned 933 installed packages from frontend/yarn.lock
findings: 664 total, 663 accepted by frontend/guarddog.ignore, 1 new
  NEW npm-lifecycle-script | guarddog-seed@1.0.0 | package.json
```

The SARIF pointed at the seed's `frontend/yarn.lock` stanza, the step exited 1, and the seed was then
removed (package manifest, lockfile, and `node_modules` entry). This proves the scanner is wired to
the installed tree, the ignore list does not swallow new packages, and the lifecycle inventory reports
the attack surface the issue named.

## Revisit if

- the advisory baseline stays green through a dependency cycle and promotion to a required check is
  worth the merge latency;
- GuardDog gains a yarn v1 lockfile/project parser that can replace the installed-tree wrapper;
- the project moves to Yarn Berry (whose lockfile format GuardDog/Semgrep would need re-evaluating);
- Socket.dev offers a self-hosted runner with no repository-access grant, or a real malware finding
  shows the behaviour rules are too noisy/too quiet to act on.
