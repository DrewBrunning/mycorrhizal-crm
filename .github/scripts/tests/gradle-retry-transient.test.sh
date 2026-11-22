#!/usr/bin/env bash
#
# Tests for ../gradle-retry-transient.sh (issue #2854).
#
# What these pin: a transient artifact-host failure (HTTP 429/5xx, a reset
# connection, a TLS handshake failure) is retried once and the leg passes,
# loudly; everything that signals a real build problem -- a compile error, a
# genuinely missing artifact, a repeat failure -- still fails the leg; the
# nightly `schedule` tier never retries; and the arguments reach Gradle
# verbatim. `./gradlew` is a stub in a scratch directory replaying scripted
# output per call: call 1 prints $FIRST_OUT and exits $FIRST_RC, every later
# call prints $RETRY_OUT and exits $RETRY_RC.

set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPT="$SCRIPT_DIR/../gradle-retry-transient.sh"

indent() { printf "    | %s\n" "${1//$'\n'/$'\n    | '}"; }

pass=0
fail=0

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
mkdir -p "$work/repo"

cat > "$work/repo/gradlew" <<'STUB'
#!/usr/bin/env bash
echo "gradlew $*" >> "$GRADLE_LOG"
calls="$(wc -l < "$GRADLE_LOG")"
if [ "$calls" -eq 1 ]; then
  printf '%s\n' "${FIRST_OUT:-}"
  exit "${FIRST_RC:-0}"
fi
printf '%s\n' "${RETRY_OUT:-}"
exit "${RETRY_RC:-0}"
STUB
chmod +x "$work/repo/gradlew"

RATE_LIMITED='* What went wrong:
Could not resolve all artifacts for configuration '"'"'classpath'"'"'.
> Could not download kotlin-stdlib-2.4.0.jar (org.jetbrains.kotlin:kotlin-stdlib:2.4.0)
   > Could not GET '"'"'https://repo.maven.apache.org/maven2/org/jetbrains/kotlin/kotlin-stdlib/2.4.0/kotlin-stdlib-2.4.0.jar'"'"'. Received status code 429 from server: Too Many Requests'
COMPILE_ERROR='e: file:///home/runner/work/android/app/src/main/Kotlin/Foo.kt:12:5 Unresolved reference: bar
FAILURE: Build failed with an exception.'
MISSING_ARTIFACT='* What went wrong:
Execution failed for task '"'"':app:compileObtainiumDebugKotlin'"'"'.
> Could not find org.example:not-published:1.2.3.
  Searched in the following locations:
    - https://repo.maven.apache.org/maven2/org/example/not-published/1.2.3/not-published-1.2.3.pom'
RESET='> Could not GET '"'"'https://plugins.gradle.org/m2/x.jar'"'"'. Connection reset'

# scenario NAME WANT_RC WANT_CALLS WANT_SUBSTRING -- ENV=VAL...
scenario() {
	local name="$1" want_rc="$2" want_calls="$3" want_out="$4"
	shift 5
	: > "$work/gradle.log"
	local out rc calls
	out="$(cd "$work/repo" && env GRADLE_LOG="$work/gradle.log" "$@" bash "$SCRIPT" --warning-mode=fail detekt 2>&1)"
	rc=$?
	calls="$(wc -l < "$work/gradle.log")"
	if [ "$rc" -eq "$want_rc" ] && [ "$calls" -eq "$want_calls" ] && grep -qF -- "$want_out" <<<"$out"; then
		pass=$((pass + 1))
		echo "PASS: $name"
	else
		fail=$((fail + 1))
		echo "FAIL: $name (rc=$rc want=$want_rc, gradle calls=$calls want=$want_calls, want substring '$want_out')"
		indent "$out"
	fi
}

scenario "a green build is one Gradle call" 0 1 "" -- FIRST_RC=0 FIRST_OUT="BUILD SUCCESSFUL"
scenario "an HTTP 429 is retried once and the leg passes with a warning" 0 2 "passed on retry" -- FIRST_RC=1 FIRST_OUT="$RATE_LIMITED" RETRY_RC=0
scenario "a reset connection is retried once" 0 2 "passed on retry" -- FIRST_RC=1 FIRST_OUT="$RESET" RETRY_RC=0
scenario "a 429 that repeats still fails the leg" 1 2 "failed again after a transient" -- FIRST_RC=1 FIRST_OUT="$RATE_LIMITED" RETRY_RC=1 RETRY_OUT="$RATE_LIMITED"
scenario "a compile error is not retried" 1 1 "non-transient reason" -- FIRST_RC=1 FIRST_OUT="$COMPILE_ERROR"
scenario "a missing artifact is not retried" 1 1 "non-transient reason" -- FIRST_RC=1 FIRST_OUT="$MISSING_ARTIFACT"
scenario "the nightly schedule tier is zero-retry" 1 1 "zero-retry" -- GITHUB_EVENT_NAME=schedule FIRST_RC=1 FIRST_OUT="$RATE_LIMITED"
scenario "a green schedule run still passes" 0 1 "" -- GITHUB_EVENT_NAME=schedule FIRST_RC=0 FIRST_OUT="BUILD SUCCESSFUL"

# The wrapper must pass its arguments through unchanged, including a value with
# a space-adjacent shell expansion the workflows rely on.
: > "$work/gradle.log"
out="$(cd "$work/repo" && env GRADLE_LOG="$work/gradle.log" FIRST_RC=0 bash "$SCRIPT" --warning-mode=fail testDebugUnitTest :app:testFossDebugUnitTest 2>&1)"
if grep -qxF "gradlew --warning-mode=fail testDebugUnitTest :app:testFossDebugUnitTest" "$work/gradle.log"; then
	pass=$((pass + 1))
	echo "PASS: Gradle arguments are passed through verbatim"
else
	fail=$((fail + 1))
	echo "FAIL: Gradle arguments were not passed through verbatim"
	indent "$(cat "$work/gradle.log")"
	indent "$out"
fi

echo
echo "passed: $pass, failed: $fail"
[ "$fail" -eq 0 ]
