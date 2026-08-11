# M12 — Cadence policy panel for Android

| | |
|---|---|
| **Rating** | 5 — T19 cadence is a rating-5 capability on web; Android has none of it |
| **Source** | [M8](89-M8-web-android-parity-audit.md) audit, 2026-08-11 |
| **Depends on** | Nothing — the T19 backend already exists and serves web today |
| **Status** | TO BE DONE |

Cadence/relationship-health (T19) has **zero Android footprint** — no screen, no ViewModel, no
repository, no route. `grep -rli cadence android/` turns up only unrelated notification-scheduling
code. This isn't a wiring gap like notes/activities; the feature was never started on Android. It
also feeds [M11](93-M11-android-prep-view.md)'s health card and
[M10](92-M10-android-dashboard-composite.md)'s overdue-cadences widget, so landing this unblocks
real content in both.

## Scope (mirrors `CadencePanel.tsx`/`CadenceDialog.tsx`)

- Set/edit cadence policy on a contact: interval (days), qualifying-interaction-type checkboxes
  (per `91.10`/`Activity.Qualifying()` — a qualifying interaction resets the cadence clock, not
  completing an unrelated task; see `/CLAUDE.md`'s domain notes).
- Delete a cadence policy.
- Health readout: overdue/on-track status, next-due date, last-interaction date.
- New `CadencePolicyRepository` in `core/domain`, a `cadence` nav route, and a screen following the
  existing per-contact sub-resource pattern (`RemindersScreen`/`RelationshipsScreen` are reasonable
  templates for structure).

## Done when

- Create/edit/delete a cadence policy from a contact on Android, round-tripping correctly with web
  (edit on one platform, see the health readout update on the other).
- Health readout matches web's overdue/on-track/next-due/last-interaction logic exactly — this is
  driven by shared backend logic, so it should be a read of the same computed fields, not a
  reimplementation.
- Hand-verified on-device: create a policy, log a qualifying activity, confirm the cadence clock
  resets (not on an unrelated completed task) per `/CLAUDE.md`'s domain note.
- New strings translated in all five locales.
