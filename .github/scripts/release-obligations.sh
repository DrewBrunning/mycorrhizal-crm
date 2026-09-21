#!/usr/bin/env bash
#
# The two final-release-only obligations, as a single definition (issue #1195).
#
# Both are deliberately skipped for an RC cut: an RC ships the same tree as the
# final, and the obligations are about the *release*, so release.yml gates them
# on `is_rc != true` and defers them to promotion (docs/release-candidate-process.md).
# Before #1195 nothing re-ran them when promote-rc.yml turned an RC into the
# final, so an RC-derived final (every release since v0.9.0) never had them
# enforced. release.yml's final path and promote-rc.yml both call this script,
# so there is one definition of each gate.
#
# Usage (run from the repo root, against the commit being released on HEAD):
#
#   release-obligations.sh asvs [--ack <reason>]
#       Require a new `| N.N | ...` changelog row in
#       docs/security/asvs-l2-verification-report.md since the previous release
#       tag, or a recorded acknowledgement.
#
#   release-obligations.sh adversarial --release <tag> [--ack <reason>] [--dry-run]
#       Classify the paths changed since the previous release tag and require
#       docs/security/adversarial-deltas.md to hold a row for the release naming
#       each security-relevant surface class it touched, or a recorded
#       acknowledgement. `--dry-run` downgrades a would-block to a warning.
#
# The compare base is the most recent release tag reachable from HEAD
# (`git describe --tags --abbrev=0 --match 'v[0-9]*'`). With no prior release tag
# both obligations are treated as a baseline and pass.
#
# Exit 0: satisfied (a row exists, no surface class touched, a recorded ack, or
# a dry-run warning). Exit 1: unsatisfied. Exit 2: usage error.

set -u

emit_error() { printf '::error::%s\n' "$1"; }
emit_warning() {
  printf '::warning::%s\n' "$1"
  # Mirror the warning into the run summary when Actions provides one.
  if [ -n "${GITHUB_STEP_SUMMARY:-}" ]; then
    printf '### :warning: %s\n' "$1" >> "$GITHUB_STEP_SUMMARY"
  fi
}

previous_tag() {
  git describe --tags --abbrev=0 --match 'v[0-9]*' HEAD 2>/dev/null || true
}

# asvs [--ack <reason>]
run_asvs() {
  local ack=""
  while [ $# -gt 0 ]; do
    case "$1" in
      --ack)
        ack="${2:-}"
        shift 2
        ;;
      *)
        echo "release-obligations.sh: unknown asvs option: $1" >&2
        return 2
        ;;
    esac
  done

  local report="docs/security/asvs-l2-verification-report.md"
  local prev
  prev="$(previous_tag)"
  if [ -z "$prev" ]; then
    echo "No previous release tag found -- treating the current report as the baseline."
    return 0
  fi
  echo "Previous release: $prev"

  # A re-verification pass adds a row to the report's section 10 changelog:
  # `| N.N | DATE | ... |`. Require at least one such added line.
  local added
  added="$(git diff "${prev}..HEAD" -- "$report" | grep -cE '^\+\|[[:space:]]*[0-9]+\.[0-9]+[[:space:]]*\|' || true)"
  if [ "$added" -gt 0 ]; then
    echo "$report carries $added new changelog row(s) since $prev."
    return 0
  fi
  if [ -n "$ack" ]; then
    emit_warning "ASVS re-verification row absent; dispatcher acknowledged current: ${ack}"
    return 0
  fi
  emit_error "$report has no new section-10 changelog row since $prev. Re-verify per section 8 and add the row, or re-dispatch with ack_asvs_current=<reason>."
  return 1
}

# adversarial --release <tag> [--ack <reason>] [--dry-run]
run_adversarial() {
  local release="" ack="" dry_run="false"
  while [ $# -gt 0 ]; do
    case "$1" in
      --release)
        release="${2:-}"
        shift 2
        ;;
      --ack)
        ack="${2:-}"
        shift 2
        ;;
      --dry-run)
        dry_run="true"
        shift
        ;;
      *)
        echo "release-obligations.sh: unknown adversarial option: $1" >&2
        return 2
        ;;
    esac
  done
  if [ -z "$release" ]; then
    echo "release-obligations.sh: adversarial requires --release <tag>" >&2
    return 2
  fi

  local prev
  prev="$(previous_tag)"
  if [ -z "$prev" ]; then
    echo "No previous release tag found -- treating the current ledger as the baseline."
    return 0
  fi
  echo "Previous release: $prev"

  # Feed cmd/adversarialdelta the paths changed since the previous tag. It
  # classifies any security-relevant surface class they touch and requires the
  # ledger to hold a row for this release naming each class; a release that
  # touched no recognised class exits 0 with nothing to record. This is the
  # per-release backstop for a new surface class none of §9's per-class checks
  # anticipated (#953).
  local out rc
  out="$(git diff --name-only "${prev}..HEAD" | (cd backend && go run ./cmd/adversarialdelta -release "$release" -ack "$ack"))"
  rc=$?
  printf '%s\n' "$out"
  if [ "$rc" -eq 0 ]; then
    return 0
  fi
  # A dry run rehearses the path; it does not release memory. A missing ledger
  # row while the register is being populated must not make the weekly
  # rehearsal red for a version that is not being cut, so a dry run records the
  # would-block as a warning and continues. A real release fails hard.
  if [ "$dry_run" = "true" ]; then
    emit_warning "Adversarial-delta gate would block a real release of $release; dry run continues. See docs/security/adversarial-deltas.md."
    return 0
  fi
  emit_error "Adversarial-delta gate failed. Record the delta in docs/security/adversarial-deltas.md, or re-dispatch with ack_adversarial_delta=<reason>."
  return 1
}

if [ $# -eq 0 ]; then
  echo "usage: release-obligations.sh {asvs|adversarial} ..." >&2
  exit 2
fi

command="$1"
shift
rc=0
case "$command" in
  asvs) run_asvs "$@" || rc=$? ;;
  adversarial) run_adversarial "$@" || rc=$? ;;
  *)
    echo "release-obligations.sh: unknown command: $command" >&2
    exit 2
    ;;
esac
exit "$rc"
