---
title: Security stewardship cadence
nav_order: 21
---

# Security stewardship cadence

This page is the written-down answer to issues
[#955](https://github.com/DrewBrunning/mycorrhizal-crm/issues/955) and
[#956](https://github.com/DrewBrunning/mycorrhizal-crm/issues/956), from the
2026-09 adversarial review: the project had rotation *procedures* (the
[incident-response runbook](incident-response.md)) but no stated rotation
*interval*, no age visibility, and no calendar backstop that would ever notice
when a root secret or a stewardship review went stale.

It is the single register of the recurring security-stewardship obligations
that are **not** already owned by another gate:

- root-secret rotation ([#955](https://github.com/DrewBrunning/mycorrhizal-crm/issues/955)),
- the Android signing-keystore custody/restore check,
- the sensitive-resource access-list review and the support-window/EOL review
  ([#956](https://github.com/DrewBrunning/mycorrhizal-crm/issues/956)).

The ASVS/MASVS re-verification calendar backstop is **not** here — it already
has its own weekly staleness job (issue #949), and the pentest engagement
cadence lives in
[the pen-test environment](../development/pentest-environment.md) (issue #946).
This page states one source per obligation rather than a second copy.

`.github/workflows/security-cadence.yml` runs `backend/cmd/securitycadence`
monthly against the register below and goes red when an entry is past its
interval. The register is a **human-maintained record**: no automation can know
when an operator last rotated `JWT_SECRET_KEY`, so the check is only as
truthful as the date you wrote down. Updating the `last_done` cell in the same
breath as performing the rotation is the whole discipline.

## Root-secret rotation intervals

Every secret below protects something that cannot be reconstructed from public
information, and every one is a single point of irreversible loss if the only
copy is lost. The interval is a ceiling, not a target: rotate immediately on a
suspected exposure, a maintainer/operator change, a compromised host, or any
time the incident-response runbook says to — the interval only answers "how
long may a healthy deployment keep the same value".

| Secret (id) | What it protects | Interval | Rotation cost |
|---|---|---|---|
| `JWT_SECRET_KEY` (`jwt_secret_key`) | Session and 2FA-challenge JWTs; as the HKDF root, all stored integration credentials and TOTP secrets; the at-rest master key when no dedicated key is set | **Annually** | High — logs everyone out and makes stored credentials/2FA unre-enterable until re-enrolled; decouple the at-rest key first (see the [runbook](incident-response.md)) |
| `DATA_ENCRYPTION_KEY` (`data_encryption_key`) | The KEK that wraps the field-encryption DEK | **Annually** | Low — `cmd/rotate-at-rest-key` rewraps the one DEK row; no payload re-encryption, no user impact |
| Release GitHub App private key (`release_app_key`) | `RELEASE_APP_ID`/`RELEASE_APP_PRIVATE_KEY`, which mints the token `release.yml` uses for the release-registration commit and tag push | **Annually** | Low — GitHub Apps support overlapping keys; add the new key, update the secret, revoke the old |
| Android signing keystore (`android_signing_key`) | The `SIGNING_*` secrets that sign release APKs | **Custody check annually; rotate only as a planned key-rotation event** | High — the signing key is **not** rotated on a schedule (see below): replacing it breaks in-place upgrade for every installed app |

The per-secret *procedure* (commands, blast radius, verification) is the
[incident-response runbook](incident-response.md); this table is the interval
the runbook did not previously state. The two are kept in step: an entry cannot
be added here without a matching procedure there, and the
`securitycadence` completeness test pins the rotation set.

## The register

The machine-readable block between the markers is what
`backend/cmd/securitycadence` parses. Each row is
`id | obligation | interval_days | last_done | action`. The `last_done` date is
the last time the obligation was actually performed, not when this page was
edited. Add a row only with its procedure/owner written down; the command fails
loudly on a malformed row rather than skipping it.

<!-- security-cadence:begin -->
| id | obligation | interval_days | last_done | action |
|---|---|---|---|---|
| `jwt_secret_key` | Rotate `JWT_SECRET_KEY` | 365 | 2026-09-18 | Follow the [runbook procedure](incident-response.md); decouple `DATA_ENCRYPTION_KEY` first or the server will not boot. |
| `data_encryption_key` | Rotate `DATA_ENCRYPTION_KEY` | 365 | 2026-09-18 | `make rotate-at-rest-key NEW=<fresh>`, persist the new value, restart. |
| `release_app_key` | Rotate the release GitHub App private key | 365 | 2026-09-18 | Add the new key in the App settings, update the Actions secret, revoke the old key. |
| `android_signing_key` | Confirm the Android signing keystore is still recoverable | 365 | 2026-09-18 | Restore the keystore from its offline backup into a scratch dir and confirm the release APK still builds; see [custody & backup](#android-signing-keystore-custody-and-backup). |
| `access_list_review` | Review the sensitive-resource access list in `GOVERNANCE.md` | 365 | 2026-09-18 | Confirm who holds repo admin, ruleset bypass, Actions secrets, and the deployment host; update the table in place ([GOVERNANCE.md](https://github.com/DrewBrunning/mycorrhizal-crm/blob/main/GOVERNANCE.md)). |
| `support_window_review` | Review the support window / EOL policy | 365 | 2026-10-26 | Confirm `SECURITY.md` "Supported Versions" and the [supported-versions lifecycle](../supported-versions.md#version-support-lifecycle) still describe what is backported, and that every supported minor has a `release/vX.Y.0` branch. |
<!-- security-cadence:end -->

## Android signing-keystore custody and backup

The Android release key is the one secret here that **must not** be rotated on a
calendar. Android refuses an in-place upgrade unless the new APK is signed by
the same key as the installed one (`INSTALL_FAILED_UPDATE_INCOMPATIBLE`), so
losing or replacing the key strands every installed app: users must uninstall
and reinstall, losing their offline mirror. Custody and recoverability are the
control, not rotation.

- **Where it lives.** The release keystore and its passwords are held as the
  `SIGNING_KEYSTORE_BASE64`, `SIGNING_KEY_ALIAS`, `SIGNING_KEY_PASSWORD`, and
  `SIGNING_STORE_PASSWORD` GitHub Actions secrets
  (`docker-publish.yml` → `build-android-apk`). The keystore
  is used only by the release workflow; no PR-triggered path can read it.
- **Offline backup.** At least one copy of the keystore file and its passwords
  exists **off GitHub**, on storage the maintainer controls and can reach if
  GitHub loses the secret or the account is locked: an offline/encrypted
  archive (password manager attachment, encrypted disk) — never the repository,
  never a plain cloud drive. Back up the keystore **and** the alias and both
  passwords; a keystore without its passwords is unrecoverable.
- **Recovery rehearsal.** The annual custody check restores the keystore from
  that backup into a scratch directory and confirms the release APK still
  builds and installs over the previous release. A backup that has never been
  restored is an assumption, not a backup.
- **Rotation, if it is ever forced** (key compromise). Rotating is a deliberate
  key-rotation event with a published notice, not a routine edit: use Android's
  [APK Signature Scheme v3 key-rotation](https://source.android.com/docs/security/features/apksigning/v3)
  lineage so a new key can update apps signed by the old one, and keep the old
  keystore available for as long as any user may still be on the old lineage.
  Record the new key's `last_done` in the register when it lands.

## Periodic reviews

Two duties in #956 have no natural event trigger and so share the register:

- **Sensitive-resource access list.** `GOVERNANCE.md` records who can publish a
  release, reach the Actions secrets, or take over infrastructure. It is
  updated on every access change, but a solo project can go a long time with no
  change and no fresh confirmation that the list is still complete. The annual
  review re-reads the live GitHub collaborators/ruleset bypass list and the App
  installations against the committed table and records the date.
- **Support window / EOL.** The canonical statement is `SECURITY.md` →
  Supported Versions. The annual review confirms it still matches what is
  actually backported.

## Support window and end-of-life

The canonical support window is `SECURITY.md` → Supported Versions, in force
since `1.0.0` (issue #956). In short: within a major, the newest minor and the
one before it are supported; the next minor release retires the older of
the two immediately, and patch releases do not change the set. When a new major
ships, the previous major's final minor gets a **12-month security-only tail**.
Only security fixes are backported.

This page owns the operational half:

- **Backport path.** A security fix is authored on `main`, then cherry-picked
  onto the `release/vX.Y.0` branch of each still-supported minor and cut from
  there (the same branches and promotion flow as
  [`release-candidate-process.md`](../release-candidate-process.md)). A fix that
  cannot be cherry-picked cleanly is re-implemented on the older branch and
  called out in the advisory.
- **EOL review.** The annual `support_window_review` entry above confirms
  `SECURITY.md` and [`supported-versions.md`](../supported-versions.md) still
  state the window accurately, and that every supported minor has a
  `release/vX.Y.0` branch. EOL branches may remain for history; they receive no
  new commits. Mechanical EOL flagging in the release workflow remains a
  separate follow-up; the window itself has one canonical home in `SECURITY.md`,
  not a second copy here.

## Updating the register

When an obligation is performed, edit its `last_done` cell to the date it was
done and open a PR. The `security-cadence` workflow then measures from the new
date. The command is also runnable locally:

```bash
cd backend && go run ./cmd/securitycadence
```

(That block needs a repo checkout with a Go toolchain — it is a maintainer
tool, not an operator one. Operators get the same information from
`GET /api/v1/admin/diagnostics`, whose `at_rest_key` check reports the age of
this instance's at-rest key material.)
