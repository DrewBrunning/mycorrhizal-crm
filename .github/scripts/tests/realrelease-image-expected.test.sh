#!/usr/bin/env bash
#
# Tests for ../realrelease-image-expected.sh (issue #1713).
#
# Pins the two rules: (1) the release-registration commit's push does NOT fail
# on a new, not-yet-published release (SKIP + ::notice::); (2) a published
# release whose image is missing still fails (RUN, so the leg's pull fails).

set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPT="$SCRIPT_DIR/../realrelease-image-expected.sh"

pass=0
fail=0

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

# A repo whose HEAD commit does (register=1) or does not (register=0) add v1.9.9.
mkrepo() {
  local register="$1" dir
  dir="$(mktemp -d -p "$work")"
  (
    cd "$dir" || exit 1
    git init -q
    git config user.email t@example.com
    git config user.name t
    git config commit.gpgsign false
    mkdir -p backend/internal/schemafixture
    printf 'var SupportedReleases = []Release{\n\t{Tag: "v1.9.8", Version: 1},\n}\n' > backend/internal/schemafixture/releases.go
    git add -A && git commit -q -m base
    if [ "$register" = 1 ]; then
      printf 'var SupportedReleases = []Release{\n\t{Tag: "v1.9.8", Version: 1},\n\t{Tag: "v1.9.9", Version: 1},\n}\n' > backend/internal/schemafixture/releases.go
    else
      echo "// unrelated" >> backend/internal/schemafixture/releases.go
    fi
    git add -A && git commit -q -m head
  )
  echo "$dir"
}

stub() { # $1 = name, $2 = exit code, $3 = stderr text
  printf '#!/usr/bin/env bash\necho "%s" >&2\nexit %s\n' "$3" "$2" > "$work/$1"
  chmod +x "$work/$1"
}
stub present 0 ""
stub unknown 1 "manifest unknown"
stub ratelimit 1 "toomanyrequests: rate limit exceeded"
stub pending1 0 ""
printf '#!/usr/bin/env bash\necho 1\n' > "$work/pending1"
printf '#!/usr/bin/env bash\necho 0\n' > "$work/pending0"
chmod +x "$work/pending1" "$work/pending0"

run_test() {
  local name="$1" register="$2" manifest="$3" pending="$4" want="$5" want_notice="$6" dir out
  dir="$(mkrepo "$register")"
  out="$(cd "$dir" && REALRELEASE_MANIFEST_CMD="$work/$manifest" REALRELEASE_PUBLISH_PENDING_CMD="$work/$pending" bash "$SCRIPT" v1.9.9 2>&1)"
  local ok=1
  [ "$(printf '%s\n' "$out" | grep -E '^(RUN|SKIP)$')" = "$want" ] || ok=0
  if [ "$want_notice" = 1 ]; then
    printf '%s' "$out" | grep -q '^::notice::' || ok=0
  else
    printf '%s' "$out" | grep -q '::notice::' && ok=0
  fi
  if [ "$ok" -eq 1 ]; then pass=$((pass + 1)); echo "PASS: $name"
  else fail=$((fail + 1)); echo "FAIL: $name (want=$want notice=$want_notice out=$out)"; fi
}

run_test "release-registration commit + image not published -> SKIP with a notice" \
  1 unknown pending0 SKIP 1
run_test "image published -> RUN, even on the registration commit" \
  1 present pending0 RUN 0
run_test "REGRESSION GUARD: later commit, publish finished, image missing -> RUN (leg then fails)" \
  0 unknown pending0 RUN 0
run_test "later commit but the tag's docker-publish is still running, image missing -> SKIP" \
  0 unknown pending1 SKIP 1
run_test "registry rate limit is not proof of absence -> RUN even on the registration commit" \
  1 ratelimit pending1 RUN 0

# The decision is also exported for the workflow step.
dir="$(mkrepo 1)"
(cd "$dir" && GITHUB_OUTPUT="$work/out" REALRELEASE_MANIFEST_CMD="$work/unknown" bash "$SCRIPT" v1.9.9 >/dev/null)
if grep -qx 'skip=true' "$work/out"; then pass=$((pass + 1)); echo "PASS: writes skip=true to GITHUB_OUTPUT"
else fail=$((fail + 1)); echo "FAIL: GITHUB_OUTPUT"; fi

echo ""
echo "$pass passed, $fail failed"
[ "$fail" -eq 0 ]
