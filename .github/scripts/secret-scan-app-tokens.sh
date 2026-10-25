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
#   - it extracts just the `mycorrhizal-app-token` rule into a standalone
#     config. gitleaks applies top-level `[[allowlists]]` (including the
#     extended default pack's path allowlists) to every rule, so reusing the
#     base file would silently hide a token committed to an allowlisted path
#     or placed on an allowlisted line. This scan deliberately inherits none;
#   - it scans the git directory rather than the worktree: gitleaks also
#     unconditionally reads `<source>/.gitleaksignore`, so scanning the
#     worktree would let a PR suppress its own finding by committing one;
#   - it passes `--ignore-gitleaks-allow`, so an inline `gitleaks:allow`
#     comment committed with the token cannot suppress it either;
#   - it walks every commit (`--log-opts=--all`), so a later deletion does not
#     hide a committed token.
#
# Fail-closed on a shallow checkout, a missing/unreadable base config, an
# unextractable rule, a missing binary, or a gitleaks finding.
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

git_dir="$(git -C "$REPO_ROOT" rev-parse --absolute-git-dir 2>/dev/null)" ||
  fail "cannot locate the git directory under $REPO_ROOT"
[ -d "$git_dir" ] ||
  fail "git directory is not a directory: $git_dir"

git -C "$REPO_ROOT" fetch --quiet origin "$DEFAULT_BRANCH" ||
  fail "could not fetch origin/$DEFAULT_BRANCH"

work_dir="$(mktemp -d)"
trap 'rm -rf "$work_dir"' EXIT
config="$work_dir/base-gitleaks.toml"
scan_config="$work_dir/app-token-gitleaks.toml"
ignore_dir="$work_dir/no-ignore"
mkdir "$ignore_dir"

git -C "$REPO_ROOT" show "origin/$DEFAULT_BRANCH:.gitleaks.toml" > "$config" ||
  fail "could not read origin/$DEFAULT_BRANCH:.gitleaks.toml"

grep -qF 'id = "mycorrhizal-app-token"' "$config" ||
  fail "origin/$DEFAULT_BRANCH:.gitleaks.toml does not define id = \"mycorrhizal-app-token\""

# Copy only the app-token rule (including any rule-scoped
# `[[rules.allowlists]]`) and drop every top-level table, so the history scan
# cannot inherit a global/default-pack allowlist that suppresses the finding.
{
  printf 'title = "mycorrhizal app-token history scan"\n\n'
  awk '
    function flush() {
      if (in_rule && buf ~ /id = "mycorrhizal-app-token"/) print buf
      in_rule = 0
      buf = ""
    }
    /^\[\[rules\]\]$/ { flush(); in_rule = 1; buf = "[[rules]]\n"; next }
    /^\[\[rules\./ { if (in_rule) buf = buf $0 "\n"; next }
    /^\[/ { flush(); next }
    in_rule { buf = buf $0 "\n" }
    END { flush() }
  ' "$config"
} > "$scan_config" || fail "could not extract the app-token rule from the base config"

grep -qF 'id = "mycorrhizal-app-token"' "$scan_config" ||
  fail "could not extract the mycorrhizal-app-token rule block from origin/$DEFAULT_BRANCH:.gitleaks.toml"
grep -qE '^[[:space:]]*regex[[:space:]]*=' "$scan_config" ||
  fail "extracted mycorrhizal-app-token rule has no regex"
if grep -qE '^\[\[allowlists?\]\]|^\[allowlist\]' "$scan_config"; then
  fail "extracted config unexpectedly retained a top-level allowlist"
fi

# Exits nonzero on any finding. --redact keeps the matched value out of the
# log. The ignore path is an empty scratch directory so a `.gitleaksignore`
# in the PR checkout is not read; --ignore-gitleaks-allow is deliberately set
# so the commit cannot opt itself out with an inline comment.
"$GITLEAKS_BIN" git "$git_dir" \
  --log-opts=--all \
  --config "$scan_config" \
  --gitleaks-ignore-path "$ignore_dir" \
  --ignore-gitleaks-allow \
  --enable-rule mycorrhizal-app-token \
  --redact -v --no-banner
