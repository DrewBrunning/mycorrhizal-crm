#!/usr/bin/env bash
#
# Tests for ../secret-scan-app-tokens.sh.
#
# Pins the contract the required `Docs & security-doc citations` job relies on:
# the scan walks every commit (`--log-opts=--all`), uses the *base branch's*
# `.gitleaks.toml` (not the PR's copy), enables only the app-token rule, and
# redacts findings. It also fails closed on a shallow checkout, an unreadable
# base config, a missing rule, a missing gitleaks binary, and a finding.
#
# `git` and `gitleaks` are stubs on PATH: the git stub answers the shallow/config
# probes and records every invocation; the gitleaks stub copies the config it was
# handed and exits with the scenario's code.

set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPT="$SCRIPT_DIR/../secret-scan-app-tokens.sh"

pass=0
fail=0

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
mkdir -p "$work/bin" "$work/repo"

cat > "$work/bin/git" <<'STUB'
#!/usr/bin/env bash
printf 'git %s\n' "$*" >> "$GIT_LOG"
case "$*" in
  *"rev-parse --is-shallow-repository"*)
    printf '%s\n' "${GIT_SHALLOW:-false}"
    exit 0
    ;;
  fetch\ *|*" fetch "*)
    exit "${GIT_FETCH_RC:-0}"
    ;;
  show\ *|*" show "*)
    if [ "${GIT_SHOW_RC:-0}" -ne 0 ]; then
      exit "${GIT_SHOW_RC}"
    fi
    printf '%s' "${GIT_CONFIG_CONTENT:-}"
    exit 0
    ;;
esac
exit 0
STUB
chmod +x "$work/bin/git"

cat > "$work/bin/gitleaks" <<'STUB'
#!/usr/bin/env bash
printf 'gitleaks %s\n' "$*" >> "$GITLEAKS_LOG"
prev=""
for arg in "$@"; do
  if [ "$prev" = "--config" ]; then
    cp "$arg" "$GITLEAKS_CONFIG_COPY" 2>/dev/null || true
  fi
  prev="$arg"
done
exit "${GITLEAKS_RC:-0}"
STUB
chmod +x "$work/bin/gitleaks"

valid_config='title = "test"
[[rules]]
id = "mycorrhizal-app-token"
regex = "mycorrhizal_(?:feed_)?[A-Za-z0-9_-]{43}"
'

indent() { printf "    | %s\n" "${1//$'\n'/$'\n    | '}"; }

# run [ENV=VAL...] -- runs the script with the stubs and resets the logs.
run() {
  : > "$work/git.log"
  : > "$work/gitleaks.log"
  : > "$work/config.copy"
  env PATH="$work/bin:$PATH" \
    GIT_LOG="$work/git.log" GITLEAKS_LOG="$work/gitleaks.log" \
    GITLEAKS_CONFIG_COPY="$work/config.copy" \
    REPO_ROOT="$work/repo" DEFAULT_BRANCH="main" GITLEAKS_BIN="$work/bin/gitleaks" \
    GIT_CONFIG_CONTENT="$valid_config" \
    "$@" bash "$SCRIPT" > "$work/out" 2>&1
}

scenario() {
  local name="$1" want_rc="$2"
  shift 2
  if run "$@"; then rc=0; else rc=$?; fi
  if [ "$rc" -eq "$want_rc" ]; then
    pass=$((pass + 1))
    echo "PASS: $name"
  else
    fail=$((fail + 1))
    echo "FAIL: $name (rc=$rc want=$want_rc)"
    indent "$(cat "$work/out")"
  fi
}

check_contains() {
  local name="$1" file="$2" want="$3"
  if grep -qF -- "$want" "$file"; then
    pass=$((pass + 1))
    echo "PASS: $name"
  else
    fail=$((fail + 1))
    echo "FAIL: $name (missing '$want')"
    indent "$(cat "$file")"
  fi
}

# Success: full history, base config fetched from origin/main, one rule, redacted.
run
rc=$?
if [ "$rc" -eq 0 ]; then
  pass=$((pass + 1)); echo "PASS: a clean full-history scan exits 0"
else
  fail=$((fail + 1)); echo "FAIL: a clean full-history scan exits 0 (rc=$rc)"; indent "$(cat "$work/out")"
fi
check_contains "the scan walks all commits" "$work/gitleaks.log" "--log-opts=--all"
check_contains "the scan enables only the app-token rule" "$work/gitleaks.log" "--enable-rule mycorrhizal-app-token"
check_contains "the scan redacts findings" "$work/gitleaks.log" "--redact"
check_contains "the scan fetches the default branch" "$work/git.log" "fetch --quiet origin main"
check_contains "the scan reads the base branch config" "$work/git.log" "show origin/main:.gitleaks.toml"
check_contains "the base config reaches gitleaks" "$work/config.copy" 'id = "mycorrhizal-app-token"'

scenario "a missing app-token rule fails closed" 1 GIT_CONFIG_CONTENT='title = "no rules"
'

scenario "a shallow checkout fails closed" 1 GIT_SHALLOW=true

scenario "an unreadable base config fails closed" 1 GIT_SHOW_RC=1

scenario "a failed base fetch fails closed" 1 GIT_FETCH_RC=1

scenario "a missing gitleaks binary fails closed" 1 GITLEAKS_BIN="$work/bin/does-not-exist"

scenario "a gitleaks finding fails the scan" 1 GITLEAKS_RC=1

: > "$work/gitleaks.log"
run GIT_SHALLOW=true
if [ -s "$work/gitleaks.log" ]; then
  fail=$((fail + 1)); echo "FAIL: a shallow checkout must not invoke gitleaks"
else
  pass=$((pass + 1)); echo "PASS: a shallow checkout does not invoke gitleaks"
fi

echo
echo "secret-scan-app-tokens: $pass passed, $fail failed"
[ "$fail" -eq 0 ]
