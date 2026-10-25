#!/usr/bin/env bash
# Issue #1877 follow-up: full-history CI scan for the app's own bearer formats.
#
# The `mycorrhizal-app-token` gitleaks rule was only applied by the local
# pre-commit hook (`gitleaks protect --staged`). A token committed with
# `--no-verify` or through the web UI was not seen by TruffleHog or Trivy
# (neither knows the `mycorrhizal_` / `mycorrhizal_feed_` format), and GitHub
# push protection cannot take custom patterns on this repository.
#
# This scan closes that gap from inside the already-required, always-on
# `Docs & security-doc citations` job:
#   - it reads `.gitleaks.toml` from `origin/<default branch>`, never the PR's
#     working-tree copy, so a PR cannot weaken or delete the rule and pass;
#   - it enables only `mycorrhizal-app-token`, avoiding the known-fake
#     `generic-api-key` findings in the existing history;
#   - it walks every commit (`--log-opts=--all`), so a later deletion does not
#     hide a committed token.
#
# Fail-closed on a shallow checkout, a missing/unreadable base config, a
# missing rule, a missing binary, or a gitleaks finding.
#
# Environment:
#   REPO_ROOT       checkout to scan (full history required)
#   DEFAULT_BRANCH  branch whose `.gitleaks.toml` is the trusted config
#   GITLEAKS_BIN    pinned gitleaks binary (built from backend/tools.mod)
set -euo pipefail

fail() {
  echo "secret-scan-app-tokens: $*" >&2
  exit 1
}

: "${REPO_ROOT:?REPO_ROOT must point at the checked-out repository}"
: "${DEFAULT_BRANCH:?DEFAULT_BRANCH must name the repository default branch}"
: "${GITLEAKS_BIN:?GITLEAKS_BIN must point at the pinned gitleaks binary}"

[ -d "$REPO_ROOT" ] || fail "REPO_ROOT is not a directory: $REPO_ROOT"
command -v "$GITLEAKS_BIN" >/dev/null 2>&1 ||
  fail "gitleaks binary not found or not executable: $GITLEAKS_BIN"

shallow="$(git -C "$REPO_ROOT" rev-parse --is-shallow-repository 2>/dev/null)" ||
  fail "cannot inspect $REPO_ROOT with git"
[ "$shallow" = "false" ] ||
  fail "checkout is shallow; fetch-depth: 0 is required for the full-history scan"

git -C "$REPO_ROOT" fetch --quiet origin "$DEFAULT_BRANCH" ||
  fail "could not fetch origin/$DEFAULT_BRANCH"

config="$(mktemp)"
trap 'rm -f "$config"' EXIT

git -C "$REPO_ROOT" show "origin/$DEFAULT_BRANCH:.gitleaks.toml" > "$config" ||
  fail "could not read origin/$DEFAULT_BRANCH:.gitleaks.toml"

grep -qF 'id = "mycorrhizal-app-token"' "$config" ||
  fail "origin/$DEFAULT_BRANCH:.gitleaks.toml does not define id = \"mycorrhizal-app-token\""

# Exits nonzero on any finding; --redact keeps the matched value out of the log.
"$GITLEAKS_BIN" git "$REPO_ROOT" \
  --log-opts=--all \
  --config "$config" \
  --enable-rule mycorrhizal-app-token \
  --redact -v --no-banner
