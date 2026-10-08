#!/usr/bin/env bash
#
# Tests for ../candidate-image.sh and ../release-image-stamp.sh (issue #1484).
#
# What these pin: the build-once release image's guard rails. A gate must pull
# the candidate BY DIGEST and refuse anything else (an empty reusable-workflow
# output, a tag, a truncated digest); and the published-vs-tested comparison
# must go red on a one-character difference. `docker` is a stub on PATH that
# records its arguments, so nothing here touches a registry.

set -u
export DOCKER_PULL_RETRY_STEP=0

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CANDIDATE="$SCRIPT_DIR/../candidate-image.sh"
STAMP="$SCRIPT_DIR/../release-image-stamp.sh"

indent() { printf "    | %s\n" "${1//$'\n'/$'\n    | '}"; }

pass=0
fail=0

D1="sha256:$(printf 'a%.0s' $(seq 1 64))"
D2="sha256:$(printf 'b%.0s' $(seq 1 64))"

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
mkdir -p "$work/bin"

# docker stub: logs argv to $DOCKER_LOG; `image inspect` prints $INSPECT_OUT.
cat > "$work/bin/docker" <<'STUB'
#!/usr/bin/env bash
echo "docker $*" >> "$DOCKER_LOG"
# DOCKER_FAIL_PULLS=N makes the first N `docker pull` calls fail (retry tests).
if [ "$1" = "pull" ] && [ -n "${DOCKER_FAIL_PULLS:-}" ]; then
  n=$(grep -c '^docker pull' "$DOCKER_LOG")
  [ "$n" -le "$DOCKER_FAIL_PULLS" ] && exit 1
fi
if [ "$1" = "image" ] && [ "$2" = "inspect" ]; then
  printf '%s\n' "${INSPECT_OUT:-[]}"
fi
exit "${DOCKER_RC:-0}"
STUB
chmod +x "$work/bin/docker"

# check NAME WANT_EXIT WANT_SUBSTRING -- cmd...  (runs with the stub on PATH)
check() {
  local name="$1" want_rc="$2" want_out="$3"
  shift 4
  : > "$work/docker.log"
  local out rc
  out="$(PATH="$work/bin:$PATH" DOCKER_LOG="$work/docker.log" "$@" 2>&1)"
  rc=$?
  if [ "$rc" -eq "$want_rc" ] && grep -qF -- "$want_out" <<<"$out"; then
    pass=$((pass + 1))
    echo "PASS: $name"
  else
    fail=$((fail + 1))
    echo "FAIL: $name (rc=$rc want=$want_rc, want substring '$want_out')"
    indent "$out"
  fi
}

# --- check-digest ---------------------------------------------------------
check "check-digest accepts a sha256 digest" 0 "" -- bash "$CANDIDATE" check-digest "$D1"
check "check-digest rejects empty" 1 "not a sha256 image digest" -- bash "$CANDIDATE" check-digest ""
check "check-digest rejects a tag" 1 "not a sha256 image digest" -- bash "$CANDIDATE" check-digest "candidate-abc123"
check "check-digest rejects a truncated digest" 1 "not a sha256 image digest" -- bash "$CANDIDATE" check-digest "sha256:abc"
check "check-digest rejects uppercase hex" 1 "not a sha256 image digest" -- bash "$CANDIDATE" check-digest "sha256:$(printf 'A%.0s' $(seq 1 64))"

# --- pull -----------------------------------------------------------------
export GITHUB_REPOSITORY="Owner/Repo"
unset CANDIDATE_IMAGE_REPO
export INSPECT_OUT="[\"ghcr.io/owner/repo@${D1}\"]"

check "pull pulls by digest from the lowercased repo and tags locally" 0 "Candidate image under test: ghcr.io/owner/repo@${D1}" -- \
  bash "$CANDIDATE" pull "$D1" mycorrhizal-crm-test:latest
if grep -qxF "docker pull --quiet ghcr.io/owner/repo@${D1}" "$work/docker.log" \
  && grep -qxF "docker tag ghcr.io/owner/repo@${D1} mycorrhizal-crm-test:latest" "$work/docker.log"; then
  pass=$((pass + 1)); echo "PASS: pull issued exactly pull-by-digest then tag"
else
  fail=$((fail + 1)); echo "FAIL: pull docker calls"; sed 's/^/    | /' "$work/docker.log"
fi

check "pull refuses a non-digest (no docker call may happen)" 1 "refusing to pull" -- \
  bash "$CANDIDATE" pull "latest" mycorrhizal-crm-test:latest
if [ -s "$work/docker.log" ]; then
  fail=$((fail + 1)); echo "FAIL: pull of a non-digest still called docker"
else
  pass=$((pass + 1)); echo "PASS: pull of a non-digest never reached docker"
fi

check "pull refuses an empty digest (unset workflow output)" 1 "refusing to pull" -- \
  bash "$CANDIDATE" pull "" mycorrhizal-crm-test:latest
check "pull needs a local tag" 2 "needs a local tag" -- bash "$CANDIDATE" pull "$D1" ""

check "pull fails when the pulled image carries a different digest" 1 "does not carry digest" -- \
  env INSPECT_OUT="[\"ghcr.io/owner/repo@${D2}\"]" bash "$CANDIDATE" pull "$D1" mycorrhizal-crm-test:latest

check "pull propagates a docker pull failure" 1 "" -- \
  env DOCKER_RC=1 bash "$CANDIDATE" pull "$D1" mycorrhizal-crm-test:latest

check "pull retries a transient pull failure and then succeeds" 0 "Candidate image under test" -- \
  env DOCKER_FAIL_PULLS=3 bash "$CANDIDATE" pull "$D1" mycorrhizal-crm-test:latest
check "pull gives up after 4 failed attempts" 1 "" -- \
  env DOCKER_FAIL_PULLS=4 bash "$CANDIDATE" pull "$D1" mycorrhizal-crm-test:latest
if [ "$(grep -c '^docker pull' "$work/docker.log")" -eq 4 ] && ! grep -q '^docker tag' "$work/docker.log"; then
  pass=$((pass + 1)); echo "PASS: exhausted retries made exactly 4 pulls and never tagged"
else
  fail=$((fail + 1)); echo "FAIL: exhausted retry docker calls"; sed 's/^/    | /' "$work/docker.log"
fi

check "pull honours CANDIDATE_IMAGE_REPO" 0 "ghcr.io/other/img@${D1}" -- \
  env CANDIDATE_IMAGE_REPO=ghcr.io/other/img INSPECT_OUT="[\"ghcr.io/other/img@${D1}\"]" bash "$CANDIDATE" pull "$D1" t:latest

check "pull without any repo source is a usage error" 2 "set CANDIDATE_IMAGE_REPO or GITHUB_REPOSITORY" -- \
  env -u GITHUB_REPOSITORY bash "$CANDIDATE" pull "$D1" t:latest

summary_file="$work/summary.md"
: > "$summary_file"
check "pull writes the digest to the step summary" 0 "" -- \
  env GITHUB_STEP_SUMMARY="$summary_file" bash "$CANDIDATE" pull "$D1" t:latest
if grep -qF "$D1" "$summary_file"; then
  pass=$((pass + 1)); echo "PASS: step summary names the candidate digest"
else
  fail=$((fail + 1)); echo "FAIL: step summary missing the digest"
fi

env_file="$work/github_env"
: > "$env_file"
check "pull exports MYCORRHIZAL_IMAGE to GITHUB_ENV" 0 "" -- \
  env GITHUB_ENV="$env_file" bash "$CANDIDATE" pull "$D1" t:latest
if grep -qxF "MYCORRHIZAL_IMAGE=ghcr.io/owner/repo@${D1}" "$env_file"; then
  pass=$((pass + 1)); echo "PASS: GITHUB_ENV carries the by-digest reference"
else
  fail=$((fail + 1)); echo "FAIL: GITHUB_ENV missing MYCORRHIZAL_IMAGE"
fi

# --- assert-match ---------------------------------------------------------
check "assert-match passes on equal digests" 0 "published digest == tested digest" -- \
  bash "$CANDIDATE" assert-match "$D1" "$D1" "all-in-one"
check "assert-match fails on a different digest" 1 "!= tested digest" -- \
  bash "$CANDIDATE" assert-match "$D1" "$D2" "all-in-one"
check "assert-match fails when the tested digest is missing" 1 "cannot compare" -- \
  bash "$CANDIDATE" assert-match "" "$D2" "all-in-one"
check "assert-match fails when the published digest is garbage" 1 "cannot compare" -- \
  bash "$CANDIDATE" assert-match "$D1" "nope" "all-in-one"

# --- usage ----------------------------------------------------------------
check "unknown subcommand is a usage error" 2 "usage:" -- bash "$CANDIDATE" frobnicate
check "no subcommand is a usage error" 2 "usage:" -- bash "$CANDIDATE"

# --- release-image-stamp.sh ----------------------------------------------
repo="$work/repo"
mkdir -p "$repo"
(
  cd "$repo" || exit 1
  git init -q .
  git config user.email t@example.com
  git config user.name t
  GIT_COMMITTER_DATE="2026-01-02T03:04:05Z" GIT_AUTHOR_DATE="2026-01-02T03:04:05Z" \
    git commit -q --allow-empty -m init
)
sha_full="$(git -C "$repo" rev-parse HEAD)"

# `env -u GITHUB_OUTPUT`: a real Actions step always exports GITHUB_OUTPUT, so
# without this the script appends there and the stdout-capturing cases below see
# nothing (they passed only on a developer machine where it is unset). The
# explicit `GITHUB_OUTPUT=` case further down sets it deliberately.
stamp() { (cd "$repo" && env -u GITHUB_OUTPUT bash "$STAMP" "$@"); }

out="$(stamp v1.2.3 2>&1)"
if grep -qx "display_version=1.2.3" <<<"$out" \
  && grep -qx "sha_full=$sha_full" <<<"$out" \
  && grep -qx "commit=${sha_full:0:12}" <<<"$out" \
  && grep -qx "sha7=${sha_full:0:7}" <<<"$out" \
  && grep -qx "created=2026-01-02T03:04:05Z" <<<"$out" \
  && grep -qx "source_date_epoch=$(date -u -d 2026-01-02T03:04:05Z +%s)" <<<"$out"; then
  pass=$((pass + 1)); echo "PASS: stamp derives every field from the commit"
else
  fail=$((fail + 1)); echo "FAIL: stamp output"; indent "$out"
fi

out="$(stamp v1.2.3-rc.4 2>&1)"
if grep -qx "display_version=1.2.3" <<<"$out"; then
  pass=$((pass + 1)); echo "PASS: an RC reports the final it will become (issue #1022)"
else
  fail=$((fail + 1)); echo "FAIL: rc display_version"; indent "$out"
fi

gh_out="$work/gh_output"
: > "$gh_out"
(cd "$repo" && GITHUB_OUTPUT="$gh_out" bash "$STAMP" v1.2.3 >/dev/null 2>&1)
if grep -qx "display_version=1.2.3" "$gh_out"; then
  pass=$((pass + 1)); echo "PASS: stamp writes to GITHUB_OUTPUT when set"
else
  fail=$((fail + 1)); echo "FAIL: GITHUB_OUTPUT not written"
fi

out="$(stamp 1.2.3 2>&1)"; rc=$?
if [ "$rc" -eq 2 ] && grep -qF "is not a release tag" <<<"$out"; then
  pass=$((pass + 1)); echo "PASS: stamp rejects a tag without the leading v"
else
  fail=$((fail + 1)); echo "FAIL: stamp bad tag (rc=$rc)"
fi
out="$(stamp 2>&1)"; rc=$?
if [ "$rc" -eq 2 ]; then
  pass=$((pass + 1)); echo "PASS: stamp requires a tag"
else
  fail=$((fail + 1)); echo "FAIL: stamp no-arg (rc=$rc)"
fi

echo
echo "candidate-image tests: ${pass} passed, ${fail} failed"
[ "$fail" -eq 0 ]
