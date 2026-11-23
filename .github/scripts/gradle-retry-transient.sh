#!/usr/bin/env bash
#
# Run a Gradle task with a bounded, loud retry when -- and only when -- the
# failure is a transient artifact-download error (issue #2854).
#
# Data: "Android APK assembly + packaging checks" reddened `main` on
# 2026-11-20 (run 44656984724). The job died in 30s, while configuring the
# included `:build-logic` build, because repo.maven.apache.org answered HTTP
# 429 ("Too Many Requests") to a burst of fresh fetches of the Kotlin 2.4.0
# plugin jars from shared runner IPs. Every other Android job in that run was
# green: nothing about the commit was wrong. The Gradle Plugin Portal redirects
# those artifacts to Maven Central, so the 429 arrives even though the build
# lists the portal as a repository.
#
# Retrying blindly would turn one broken build into several: the assemble steps
# take 10+ minutes each. So this wrapper retries ONLY when the captured output
# carries a transport-level signature -- HTTP 429/5xx from the artifact host, a
# reset/refused connection, a DNS/TLS handshake failure, a truncated body.
# A compile error, a failing test, a genuinely missing artifact ("Could not
# find ...", a 404) and every other deterministic failure exit on the FIRST
# attempt with Gradle's own status, unchanged.
#
# The host can be reached both through Gradle's resolver ("Received status code
# 429 from server") and through Robolectric's runtime jar fetch ("Server
# returned HTTP response code: 429 for URL"); both are matched.
#
# The retry is not immediate: a 429 is a rate limit, so a same-second retry
# from the same runner IP almost always earns another 429 (the 2026-11-23
# push runs on `main` did exactly that, 12s apart, and a 30s wait still lost
# `buildHealth` to the same throttle). Back off exponentially
# (60s, then 120s, capped at 300s) before each retry; a transient blip is over
# by then, and a deterministic failure still exits on its first attempt.
#
# The nightly `schedule` tier is zero-retry on purpose -- a red nightly IS the
# flake signal (docs/development/testing.md, "Nightly failure alerting") -- so
# on that event the first failure is reported as-is.
#
# Usage, from the android/ directory (the wrapper is ./gradlew):
#   "$GITHUB_WORKSPACE/.github/scripts/gradle-retry-transient.sh" <gradle args...>
#
# Env: GRADLE_RETRY_MAX_ATTEMPTS (default 3 -- two retries),
#      GRADLE_RETRY_DELAY_SECONDS (default 60 -- doubles per retry, capped at 300),
#      GITHUB_EVENT_NAME.
set -uo pipefail

attempts="${GRADLE_RETRY_MAX_ATTEMPTS:-3}"
case "$attempts" in
'' | *[!0-9]*)
	echo "::error::GRADLE_RETRY_MAX_ATTEMPTS must be a positive integer (got '$attempts')" >&2
	exit 2
	;;
esac
if [ "$attempts" -lt 1 ]; then
	echo "::error::GRADLE_RETRY_MAX_ATTEMPTS must be >= 1 (got '$attempts')" >&2
	exit 2
fi

delay="${GRADLE_RETRY_DELAY_SECONDS:-60}"
case "$delay" in
'' | *[!0-9]*)
	echo "::error::GRADLE_RETRY_DELAY_SECONDS must be a non-negative integer (got '$delay')" >&2
	exit 2
	;;
esac

# Deliberately narrow: each alternative names a transport failure, never a
# build failure. "Could not resolve/Could not find" is NOT here -- that is the
# signature of a missing or misconfigured artifact, which a retry cannot fix.
transient_re='status code (429|5[0-9][0-9]) from server'
# Robolectric fetches its android-all jar outside Gradle's dependency
# resolution, so a throttled fetch surfaces as Java's wording instead:
# "Server returned HTTP response code: 429 for URL: https://repo1.maven.org/..."
# (issue #2886). Same transport failure, different text.
transient_re+='|Server returned HTTP response code: (429|5[0-9][0-9])'
transient_re+='|connection reset|connection refused|broken pipe'
transient_re+='|(read|connect) timed out|timed out waiting for'
transient_re+='|remote host terminated the handshake|unexpected end of stream'
transient_re+='|premature end of content-length delimited message body'
transient_re+='|could not install gradle distribution'
transient_re+='|unknownhostexception|pkix path building failed|sslhandshakeexception'

out="$(mktemp)"
trap 'rm -f "$out"' EXIT

# Streams Gradle's own output through, and returns GRADLE's status rather than
# tee's (pipefail alone would also fold in tee's).
run_gradle() {
	./gradlew "$@" 2>&1 | tee "$out"
	return "${PIPESTATUS[0]}"
}

attempt=1
run_gradle "$@"
rc=$?
while [ "$rc" -ne 0 ] && [ "$attempt" -lt "$attempts" ]; do
	if [ "${GITHUB_EVENT_NAME:-}" = "schedule" ]; then
		echo "::error::./gradlew $* failed on the nightly schedule run; that tier is zero-retry (issue #2854)" >&2
		exit "$rc"
	fi
	if ! grep -qiE "$transient_re" "$out"; then
		echo "::error::./gradlew $* failed for a non-transient reason; not retried (issue #2854)" >&2
		exit "$rc"
	fi
	attempt=$((attempt + 1))
	backoff=$((delay * (1 << (attempt - 2))))
	[ "$backoff" -gt 300 ] && backoff=300
	echo "::warning::transient artifact-download failure in './gradlew $*' -- waiting ${backoff}s before retrying (attempt ${attempt}/${attempts}, issue #2854)" >&2
	grep -iE "$transient_re" "$out" | head -3 >&2 || true
	[ "$backoff" -gt 0 ] && sleep "$backoff"
	run_gradle "$@"
	rc=$?
done

if [ "$rc" -ne 0 ]; then
	echo "::error::./gradlew $* failed again after a transient artifact-download error -- the artifact host is degraded, not a one-off flake (issue #2854)" >&2
	exit "$rc"
fi
if [ "$attempt" -gt 1 ]; then
	echo "::warning::FLAKE: './gradlew $*' failed on a transient artifact download and passed on retry (issue #2854). Triage; the check is not blocked." >&2
fi
exit 0
