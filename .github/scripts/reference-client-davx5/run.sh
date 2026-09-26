#!/usr/bin/env bash
# Issue #917: the automated leg of the reference-client interop matrix for
# DAVx5 — "the closest to scriptable" client per
# docs/development/reference-client-matrix.md. Drives a REAL DAVx5 APK
# (sideloaded, not our own instrumented code) through account setup against
# our REAL server via adb + uiautomator, then asserts synced data actually
# landed in Android's real ContactsContract — the same surface a human tester
# would eyeball.
#
# DAVx5 is a Jetpack Compose app with no resource-ids exposed to the
# accessibility tree, so every UI step below locates elements by exact text
# or content-desc via uiautomator's dump, not hardcoded pixel coordinates —
# resilient to minor layout shifts across DAVx5 releases (a version bump
# still requires re-verifying this script, the same way any pinned-version
# bump in this repo does).
#
# Requires: adb on PATH, a booted+unlocked emulator/device already selected
# by adb (this script does not boot one — the caller, typically
# reactivecircus/android-emulator-runner, does that).
#
# Usage:
#   SERVER_URL=http://127.0.0.1:7300 USERNAME=pentest PASSWORD=... \
#     ./run.sh /path/to/davx5.apk
set -euo pipefail

APK_PATH="${1:?usage: run.sh <path-to-davx5.apk>}"
SERVER_URL="${SERVER_URL:-http://127.0.0.1:7300}"
USERNAME="${USERNAME:?USERNAME env var required}"
PASSWORD="${PASSWORD:?PASSWORD env var required}"
PACKAGE="at.bitfire.davdroid"
# Fixed (not mktemp) so a CI step can find and upload this directory as a
# failure-diagnostics artifact after this script exits.
WORKDIR="${DAVX5_WORKDIR:-$(mktemp -d)}"
mkdir -p "$WORKDIR"
DUMP_XML="$WORKDIR/dump.xml"

# Screenshot + UI dump + the app's own logcat, for postmortem diagnosis of a
# step that fails with no single missing element to blame (a slow carousel,
# an unexpected system dialog, a crash). Safe to call multiple times; each
# call overwrites, callers pass a distinct $1 tag to keep multiple failures
# in one run.
capture_failure_diagnostics() {
	local tag="$1"
	adb exec-out screencap -p >"$WORKDIR/failure-$tag.png" 2>/dev/null || true
	cp "$DUMP_XML" "$WORKDIR/failure-dump-$tag.xml" 2>/dev/null || true
	# `-s "$PACKAGE:*"` filters logcat by TAG, but $PACKAGE
	# ("at.bitfire.davdroid") is a package name, not a logcat tag — this
	# always matched nothing and silently produced a 0-byte file (confirmed
	# on a real CI failure, where it left this diagnostic empty for exactly
	# the run that needed it). Dump the tail of the full unfiltered buffer
	# instead: the signal this script actually needs (ANR entries from
	# system_server/ActivityManager, per dismiss_anr_if_present's own
	# comment) doesn't come from the app's own tag anyway.
	adb logcat -d -t 2000 >"$WORKDIR/failure-logcat-$tag.txt" 2>/dev/null || true
}

log() { echo "[davx5-interop] $*" >&2; }

dump_ui() {
	adb shell uiautomator dump /sdcard/dump.xml >/dev/null 2>&1
	adb pull /sdcard/dump.xml "$DUMP_XML" >/dev/null 2>&1
}

# Prints "x y" (tap-able center) for the first node whose text or
# content-desc exactly matches $1, or nothing if not found.
find_center() {
	python3 - "$DUMP_XML" "$1" <<'PY'
import sys, re
import xml.etree.ElementTree as ET

path, target = sys.argv[1], sys.argv[2]
tree = ET.parse(path)
for node in tree.iter("node"):
    if node.get("text") == target or node.get("content-desc") == target:
        b = node.get("bounds")
        x0, y0, x1, y1 = map(int, re.findall(r"-?\d+", b))
        print(f"{(x0 + x1) // 2} {(y0 + y1) // 2}")
        break
PY
}

# Prints "<button><TAB><title>" for how to clear the ANR dialog currently in
# $DUMP_XML, or nothing when there is no ANR dialog. Pure (reads only the
# dump) so the decision is unit-testable without an emulator — see
# .github/scripts/tests/reference-client-davx5.test.sh.
#
# The dialog's title names the wedged process. For an app other than the one
# under test — in practice "Pixel Launcher isn't responding", the recurring
# CI failure under the runner's CPU contention — tapping "Wait" does NOT let
# it recover: a process that is genuinely starved re-ANRs within seconds, so
# every subsequent tap lands on the dialog again and the login-form loop burns
# its whole budget (observed: all 8 attempts, with "Sending oneway calls to
# frozen process" in logcat throughout). "Close app" kills the wedged process
# instead, which clears the dialog for good; the system restarts the component
# if anything still needs it, and DAVx5 — the foreground activity, a different
# process — is unaffected. Reserve "Wait" for DAVx5's own ANR, where closing
# would kill the app under test.
anr_dialog_decision() {
	python3 - "$DUMP_XML" <<'PY'
import sys
import xml.etree.ElementTree as ET

tree = ET.parse(sys.argv[1])
title = ""
has_wait = has_close = False
for node in tree.iter("node"):
    if node.get("resource-id") == "android:id/alertTitle":
        title = node.get("text") or ""
    text = node.get("text") or ""
    if text == "Wait":
        has_wait = True
    elif text == "Close app":
        has_close = True
if "isn't responding" not in title:
    sys.exit(0)
if "DAVx" in title and has_wait:
    print(f"Wait\t{title}")
elif has_close:
    print(f"Close app\t{title}")
elif has_wait:
    print(f"Wait\t{title}")
PY
}

# Diagnostic capture from a real CI failure: an "isn't responding" ANR
# dialog for **Pixel Launcher itself** (not DAVx5), with "Close app" / "Wait"
# buttons, showed up mid-carousel under a resource-starved emulator — pure
# OS-level contention, unrelated to anything DAVx5 or our server does. It
# blocks all input to the app under test until dismissed, so every tap in
# the calling loop silently lands on the dialog instead of the intended
# target. Call after dump_ui in a polling loop; anr_dialog_decision picks the
# button that actually clears the dialog (see its comment).
dismiss_anr_if_present() {
	local decision button title coords
	decision="$(anr_dialog_decision)"
	[ -n "$decision" ] || return 0
	button="${decision%%$'\t'*}"
	title="${decision#*$'\t'}"
	coords="$(find_center "$button")"
	[ -n "$coords" ] || return 0
	log "WARNING: ANR dialog ($title), tapping '$button' to clear it"
	# shellcheck disable=SC2086
	adb shell input tap $coords
	sleep 2
	dump_ui
}

# Taps the element whose text/content-desc exactly matches $1. Fails loudly
# if it isn't on screen — a silent miss (e.g. from a coordinate guess) is
# exactly the class of bug this script exists to avoid.
tap() {
	dump_ui
	dismiss_anr_if_present
	local coords
	coords="$(find_center "$1")"
	if [ -z "$coords" ]; then
		log "ERROR: could not find tappable element with text/content-desc '$1'"
		capture_failure_diagnostics "tap-$1"
		return 1
	fi
	# shellcheck disable=SC2086
	adb shell input tap $coords
	sleep 1
}

# Taps $1 repeatedly until $2 is found on screen (bounded by $3 attempts,
# 1s apart), for the multi-page DAVx5 intro carousel whose page count varies
# with permission/notification state. Does nothing if $2 is already present.
tap_until_visible() {
	local tap_target="$1" wait_for="$2" max_attempts="${3:-10}" attempts=0
	while [ "$attempts" -lt "$max_attempts" ]; do
		dump_ui
		dismiss_anr_if_present
		if [ -n "$(find_center "$wait_for")" ]; then
			return 0
		fi
		local coords
		coords="$(find_center "$tap_target")"
		if [ -n "$coords" ]; then
			# shellcheck disable=SC2086
			adb shell input tap $coords
		fi
		sleep 1
		attempts=$((attempts + 1))
	done
	log "ERROR: '$wait_for' never appeared after tapping '$tap_target' $attempts times"
	capture_failure_diagnostics "tap-until-visible-$wait_for"
	return 1
}

# Polls (no tapping) until $1 is found on screen, bounded by $2 attempts, 1s
# apart — for waiting out async work (service detection, sync) with no
# button to press in the meantime.
wait_for() {
	local wait_for="$1" max_attempts="${2:-20}" attempts=0
	while [ "$attempts" -lt "$max_attempts" ]; do
		dump_ui
		dismiss_anr_if_present
		if [ -n "$(find_center "$wait_for")" ]; then
			return 0
		fi
		sleep 1
		attempts=$((attempts + 1))
	done
	log "ERROR: '$wait_for' never appeared after ${max_attempts}s"
	capture_failure_diagnostics "wait-for-$wait_for"
	return 1
}

# Prints one "x y" line per android.widget.EditText node in $DUMP_XML, in
# document order (Base URL, User name, Password on the login screen).
find_edit_text_fields() {
	python3 - "$DUMP_XML" <<'PY'
import sys, re
import xml.etree.ElementTree as ET

tree = ET.parse(sys.argv[1])
for node in tree.iter("node"):
    if node.get("class") == "android.widget.EditText":
        b = node.get("bounds")
        x0, y0, x1, y1 = map(int, re.findall(r"-?\d+", b))
        print(f"{(x0 + x1) // 2} {(y0 + y1) // 2}")
PY
}

# Prints the current `text` value of the Nth (0-based, document order)
# android.widget.EditText node in $DUMP_XML, or nothing if there is no such
# node. Used to verify a value actually landed in the field it was meant
# for, not merely that some typing happened somewhere.
edit_text_value_at() {
	python3 - "$DUMP_XML" "$1" <<'PY'
import sys
import xml.etree.ElementTree as ET

path, index = sys.argv[1], int(sys.argv[2])
tree = ET.parse(path)
fields = [n for n in tree.iter("node") if n.get("class") == "android.widget.EditText"]
if index < len(fields):
    print(fields[index].get("text") or "")
PY
}

# Polls (no tapping, like wait_for) until 3 EditText fields are on screen,
# bounded by $1 attempts, 1s apart. Diagnostic capture from a real CI
# failure: the login screen's own dump_ui was a single unretried call with
# no dismiss_anr_if_present — an ANR dialog (observed: "Pixel Launcher isn't
# responding", stacked on top of the login form after the carousel/Continue
# taps above already dismissed two others under the same load) hid every
# EditText node from the accessibility tree, and the script failed
# immediately reading a dump it never gave a chance to recover from.
wait_for_login_fields() {
	local max_attempts="${1:-20}" attempts=0
	while [ "$attempts" -lt "$max_attempts" ]; do
		dump_ui
		dismiss_anr_if_present
		mapfile -t FIELD_COORDS < <(find_edit_text_fields)
		if [ "${#FIELD_COORDS[@]}" -ge 3 ]; then
			return 0
		fi
		sleep 1
		attempts=$((attempts + 1))
	done
	log "ERROR: expected 3 login EditText fields (base URL, user name, password), found ${#FIELD_COORDS[@]} after ${attempts}s"
	capture_failure_diagnostics "login-fields"
	return 1
}

# Clears whatever is already in the EditText at document-order index $1 (0 =
# Base URL, 1 = User name, 2 = Password) and types $2 into it. Re-dumps and
# dismisses any ANR dialog immediately before locating the field — every
# other interaction in this script does the same before acting, but the
# three field-fill calls this replaces used to reuse coordinates captured
# once, before any of the three taps. Diagnostic capture from a real CI
# failure: an ANR dialog interposed between two of those blind taps, so the
# password ended up typed into the Base URL field while User name and
# Password were left empty — login was never actually attempted, and the
# script spent its whole budget waiting for a "Finish" screen that could
# never appear. Clears the field first (move to end, then backspace past any
# plausible existing content) so a retry from fill_login_form's outer loop
# can't append onto stray text a previous failed attempt left behind.
type_login_field() {
	local field_index="$1" text="$2"
	dump_ui
	dismiss_anr_if_present
	local coords
	coords="$(find_edit_text_fields | sed -n "$((field_index + 1))p")"
	if [ -z "$coords" ]; then
		log "WARNING: login field #$field_index not on screen"
		return 0
	fi
	# shellcheck disable=SC2086
	adb shell input tap $coords
	sleep 1
	adb shell input keyevent KEYCODE_MOVE_END
	# shellcheck disable=SC2046
	adb shell input keyevent $(printf 'KEYCODE_DEL %.0s' $(seq 1 80))
	adb shell input text "$text"
	sleep 1
}

# Fills the three login EditText fields and verifies the values actually
# landed in the fields they were meant for before returning — Base URL and
# User name are checked for an exact match (both are plain-text fields, so
# this is exactly the check that would have caught the real failure
# described on type_login_field above); Password is only checked for
# non-emptiness since DAVx5 may or may not expose a masked field's real
# value to the accessibility tree. Retries the whole sequence (bounded by
# $1, default 8) rather than a single field, since the failure this guards
# against is cross-field contamination — a per-field check can't detect its
# own value ending up in the WRONG field, only that the intended field looks
# right in isolation.
#
# Diagnostic capture from a real CI failure (run 35148449853, the very first
# run of the verify-and-retry logic above): every one of 4 attempts hit an
# ANR dialog, exhausting a budget of 4 — the captured logcat showed
# "Sending oneway calls to frozen process" throughout, i.e. the emulator's
# app-freezer under sustained resource pressure, not a one-off blip. Doubled
# to 8 to match this file's existing doubling convention for exactly this
# failure mode (see "Next"/"Add account" and "synchronize this collection"
# above).
fill_login_form() {
	local max_attempts="${1:-8}" attempt=0
	while [ "$attempt" -lt "$max_attempts" ]; do
		wait_for_login_fields 20

		type_login_field 0 "$SERVER_URL"
		type_login_field 1 "$USERNAME"
		type_login_field 2 "$PASSWORD"

		# Re-check for an ANR dialog before reading the field values back: a
		# dialog that appeared during the three fills hides every EditText
		# from the accessibility tree, so the verification below would read
		# empty values and fail the attempt even though the text did land.
		# (This was the shape of the 2026-09-26 nightly failure.)
		dump_ui
		dismiss_anr_if_present
		if [ "$(edit_text_value_at 0)" = "$SERVER_URL" ] \
			&& [ "$(edit_text_value_at 1)" = "$USERNAME" ] \
			&& [ -n "$(edit_text_value_at 2)" ]; then
			return 0
		fi
		attempt=$((attempt + 1))
		log "WARNING: login fields did not contain the expected values after filling (attempt $attempt/$max_attempts), retrying"
	done
	log "ERROR: could not fill the login form correctly after $max_attempts attempts"
	capture_failure_diagnostics "fill-login-form"
	return 1
}

log "Installing DAVx5 from $APK_PATH"
adb install -r "$APK_PATH"

log "Granting contacts/calendar/notification permissions up front (skips the in-app permission wizard)"
for perm in READ_CONTACTS WRITE_CONTACTS READ_CALENDAR WRITE_CALENDAR GET_ACCOUNTS POST_NOTIFICATIONS; do
	adb shell pm grant "$PACKAGE" "android.permission.$perm" || true
done

log "Launching DAVx5"
adb shell monkey -p "$PACKAGE" -c android.intent.category.LAUNCHER 1 >/dev/null 2>&1
sleep 3

log "Walking the intro carousel to the account list"
# 15 attempts (each attempt is a full uiautomator dump + pull + a 1s sleep,
# so meaningfully more than 15s of wall clock) was tight enough that this
# step failed 3/3 times on push:main's much larger concurrent CI load while
# passing 3/3 on the lighter-weight pull_request/workflow_dispatch triggers
# — the same emulator, same pinned APK, just slower under load. Doubled to
# match the budget "Finish" below already uses for the same reason.
tap_until_visible "Next" "Add account" 30

log "Starting 'Add account'"
tap "Add account"

log "Selecting 'Login with URL and user name' and continuing"
# The tap above only fires the transition; under CI resource contention
# (same class of flake already hardened for the intro carousel and the
# post-account-creation "Finish" step above/below — see their comments) the
# next screen can take longer than tap()'s fixed 1s sleep to render, so a
# dump taken immediately after still shows the previous screen. Wait for the
# target screen first, same pattern as those other steps. Already the
# default-selected radio option once this screen is up, but tap it
# explicitly so this script doesn't depend on that default surviving a
# DAVx5 update.
# 60 attempts, not 20: the intro carousel and this screen arrive slowly on an
# emulator that is itself competing for a busy host (the RC2 flake was a Pixel
# Launcher ANR here -- issue #1174). wait_for dismisses the ANR and keeps
# polling, so the extra headroom is free on a healthy run.
wait_for "Login with URL and user name" 60
tap "Login with URL and user name"
tap "Continue"

log "Filling in server URL / username / password"
# The three EditText fields are located by their bounds order (Base URL,
# User name, Password) since Compose text fields carry no stable
# text/content-desc before they're filled in. fill_login_form re-locates and
# re-checks for an ANR dialog before every individual field (not just once
# up front) and verifies the values actually landed correctly — see its own
# comment and type_login_field's for the real CI failure this fixes.
fill_login_form 8

log "Dismissing keyboard and logging in"
adb shell input keyevent KEYCODE_BACK
sleep 1
tap "Login"

log "Waiting for service detection to finish"
wait_for "Finish" 30
tap "Finish"

log "Account created. Enabling CardDAV + CalDAV collection sync"
# Diagnostic capture from a real CI failure: switching tabs kicks off an
# async collection-discovery PROPFIND against our server, and the very next
# tap used to fire before that finished — the captured UI dump showed the
# CardDAV tab active with no collection row on screen yet, just the account
# chrome (Synchronize now / Refresh list / Options menu). wait_for is this
# script's existing tool for exactly this ("waiting out async work... with
# no button to press in the meantime") — it just wasn't used here.
#
# 20 attempts wasn't enough under load: a second real CI failure (this one
# past that first race) showed the server-side PROPFIND chain completing
# (every request 207, confirmed from the server's own request log) over a
# minute before the client's collection row ever rendered — each wait_for
# attempt's own dump_ui (two adb round trips) got slow enough under the same
# emulator contention documented elsewhere in this file that 20 attempts
# used up far more than 20 wall-clock seconds and still wasn't enough.
# Doubled to 40 to match this file's existing doubling convention for this
# exact failure mode (see "Next"/"Add account" above).
tap "CardDAV"
wait_for "synchronize this collection" 40
tap "synchronize this collection"
tap "CalDAV"
wait_for "synchronize this collection" 40
tap "synchronize this collection"

log "Triggering a manual sync"
tap "Synchronize now"
sleep 20

log "Verifying synced contacts landed in the real Android ContactsContract"
CONTACT_COUNT="$(adb shell content query --uri content://com.android.contacts/contacts --projection display_name 2>/dev/null | wc -l)"
log "ContactsContract row count: $CONTACT_COUNT"
if [ "$CONTACT_COUNT" -lt 5 ]; then
	log "ERROR: expected at least 5 synced contacts, found $CONTACT_COUNT — sync did not deliver data to the provider"
	adb exec-out screencap -p >"$WORKDIR/failure-final.png" || true
	exit 1
fi

# Non-ASCII names from the canonical pathological fixture (issue #430) —
# a stronger assertion than a bare row count: proves vCard N/FN round-trip
# through a real client's parser and Android's provider without mangling
# multi-byte / RTL text, not just that "some rows" arrived.
MISSING=0
for name in "王芳" "佐藤直樹" "יעל כהן"; do
	if ! adb shell content query --uri content://com.android.contacts/contacts --projection display_name 2>/dev/null | grep -qF "$name"; then
		log "ERROR: expected contact '$name' not found in ContactsContract after sync"
		MISSING=1
	fi
done
if [ "$MISSING" -ne 0 ]; then
	adb exec-out screencap -p >"$WORKDIR/failure-final.png" || true
	exit 1
fi

log "PASS: DAVx5 discovered, synced, and Android's contacts provider has the expected pathological-fixture data"
