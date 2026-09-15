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
	adb logcat -d -s "$PACKAGE:*" >"$WORKDIR/failure-logcat-$tag.txt" 2>/dev/null || true
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

# Diagnostic capture from a real CI failure: an "isn't responding" ANR
# dialog for **Pixel Launcher itself** (not DAVx5), with "Close app" / "Wait"
# buttons, showed up mid-carousel under a resource-starved emulator — pure
# OS-level contention, unrelated to anything DAVx5 or our server does. It
# blocks all input to the app under test until dismissed, so every tap in
# the calling loop silently lands on the dialog instead of the intended
# target. Call after dump_ui in a polling loop; tapping "Wait" lets the
# stalled process recover instead of burning the loop's whole retry budget
# against a dialog that was never going to go away on its own.
dismiss_anr_if_present() {
	if grep -q "isn't responding" "$DUMP_XML" 2>/dev/null; then
		local coords
		coords="$(find_center "Wait")"
		if [ -n "$coords" ]; then
			log "WARNING: system ANR dialog detected, tapping 'Wait' to let it recover"
			# shellcheck disable=SC2086
			adb shell input tap $coords
			sleep 2
			dump_ui
		fi
	fi
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

type_into_field_at() {
	local x="$1" y="$2" text="$3"
	adb shell input tap "$x" "$y"
	sleep 1
	adb shell input text "$text"
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
# Already the default-selected radio option, but tap it explicitly so this
# script doesn't depend on that default surviving a DAVx5 update.
tap "Login with URL and user name"
tap "Continue"

log "Filling in server URL / username / password"
dump_ui
# The three EditText fields are located by their bounds order (Base URL,
# User name, Password) since Compose text fields carry no stable
# text/content-desc before they're filled in.
python3 - "$DUMP_XML" <<'PY' >"$WORKDIR/fields.txt"
import sys, re
import xml.etree.ElementTree as ET

tree = ET.parse(sys.argv[1])
fields = []
for node in tree.iter("node"):
    if node.get("class") == "android.widget.EditText":
        b = node.get("bounds")
        x0, y0, x1, y1 = map(int, re.findall(r"-?\d+", b))
        fields.append(((x0 + x1) // 2, (y0 + y1) // 2))
for x, y in fields:
    print(x, y)
PY

mapfile -t FIELD_COORDS <"$WORKDIR/fields.txt"
if [ "${#FIELD_COORDS[@]}" -lt 3 ]; then
	log "ERROR: expected 3 login EditText fields (base URL, user name, password), found ${#FIELD_COORDS[@]}"
	exit 1
fi

# shellcheck disable=SC2086
type_into_field_at ${FIELD_COORDS[0]} "$SERVER_URL"
# shellcheck disable=SC2086
type_into_field_at ${FIELD_COORDS[1]} "$USERNAME"
# shellcheck disable=SC2086
type_into_field_at ${FIELD_COORDS[2]} "$PASSWORD"

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
tap "CardDAV"
wait_for "synchronize this collection" 20
tap "synchronize this collection"
tap "CalDAV"
wait_for "synchronize this collection" 20
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
