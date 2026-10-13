#!/usr/bin/env bash
# nightly-failure-reconcile.yml's dedupe check: has this failed scheduled run
# already been reported -- by nightly-failure-alert.yml's event-driven path,
# or by a previous day's reconciliation pass? Writes `reported=true|false` to
# $GITHUB_OUTPUT so an issue that is still open because the failure is still
# unfixed doesn't get a fresh "still broken" comment every single day.
#
# Required env: GH_REPO, GH_TOKEN, WORKFLOW_NAME, RUN_ID, GITHUB_OUTPUT.
set -euo pipefail

: "${GH_REPO:?GH_REPO required}"
: "${GH_TOKEN:?GH_TOKEN required}"
: "${WORKFLOW_NAME:?WORKFLOW_NAME required}"
: "${RUN_ID:?RUN_ID required}"
: "${GITHUB_OUTPUT:?GITHUB_OUTPUT required}"
export GH_TOKEN GH_REPO

# shellcheck source=SCRIPTDIR/../alert-lib/issues.sh
source "$(dirname "${BASH_SOURCE[0]}")/../alert-lib/issues.sh"

# Same title ensure-alert.sh files the run under.
if alert_run_reported "Nightly failure: ${WORKFLOW_NAME}" "$RUN_ID"; then
	echo "Run ${RUN_ID} is already referenced by its nightly-failure issue -- already reported."
	echo "reported=true" >>"$GITHUB_OUTPUT"
else
	echo "reported=false" >>"$GITHUB_OUTPUT"
fi
