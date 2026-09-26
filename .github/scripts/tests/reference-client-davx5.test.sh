#!/usr/bin/env bash
#
# Tests for ../reference-client-davx5/run.sh's pure ANR-dialog decision
# (anr_dialog_decision). The 2026-09-26 nightly failure was a "Pixel Launcher
# isn't responding" ANR that the old code answered by tapping "Wait" ~40 times
# over four minutes without it ever clearing; this pins the classification that
# closes a wedged non-DAVx5 process instead.
#
# run.sh's emulator flow can't run here (no adb/emulator), so the function is
# extracted from the file and evaluated on its own — it reads only the dump at
# $DUMP_XML and calls only python3.

set -u

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPT="$SCRIPT_DIR/../reference-client-davx5/run.sh"

pass=0
fail=0

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
DUMP_XML="$tmp/dump.xml"
export DUMP_XML

# Pull just the function (its own line range) out of the script so sourcing it
# doesn't execute the install/emulator flow at the bottom.
eval "$(sed -n '/^anr_dialog_decision() {/,/^}/p' "$SCRIPT")"
if ! declare -F anr_dialog_decision >/dev/null; then
	echo "FAIL: could not extract anr_dialog_decision from $SCRIPT" >&2
	exit 1
fi

anr_dump() { # <title> <has-close:0|1> <has-wait:0|1>
	local title="$1" close="$2" wait="$3" xml=""
	xml='<?xml version="1.0" encoding="UTF-8"?><hierarchy>'
	xml+="<node resource-id=\"android:id/alertTitle\" text=\"$title\"/>"
	[ "$close" = "1" ] && xml+='<node text="Close app"/>'
	[ "$wait" = "1" ] && xml+='<node text="Wait"/>'
	xml+='</hierarchy>'
	printf '%s' "$xml" >"$DUMP_XML"
}

assert_case() { # <name> <want> <title> <close> <wait>
	local name="$1" want="$2"
	shift 2
	anr_dump "$@"
	local got
	got="$(anr_dialog_decision)"
	if [ "$got" = "$want" ]; then
		pass=$((pass + 1))
		echo "PASS: $name"
	else
		fail=$((fail + 1))
		echo "FAIL: $name (got '$got' want '$want')"
	fi
}

# A wedged launcher: only "Close app" clears it for good.
assert_case "launcher ANR -> Close app" \
	$'Close app\tPixel Launcher isn\'t responding' \
	"Pixel Launcher isn't responding" 1 1

# The app under test: keep it alive so the account flow can continue.
assert_case "DAVx5 ANR -> Wait" \
	$'Wait\tDAVx⁵ isn\'t responding' \
	"DAVx⁵ isn't responding" 1 1

# Defensive: a non-DAVx5 ANR with only a Wait button still gets Wait.
assert_case "non-DAVx5 ANR, no Close app -> Wait" \
	$'Wait\tSystem UI isn\'t responding' \
	"System UI isn't responding" 0 1

# No dialog -> no decision.
assert_case "no ANR dialog -> empty" \
	"" \
	"Contacts" 0 0

# Title present but not an ANR -> no decision.
assert_case "non-ANR title -> empty" \
	"" \
	"Pixel Launcher" 1 1

echo
echo "reference-client-davx5.test.sh: $pass passed, $fail failed"
[ "$fail" -eq 0 ]
