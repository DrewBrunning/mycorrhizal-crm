#!/usr/bin/env bash
# Reconcile `token-rotation` issues with the security-cadence register.
#
# Input: the JSON array from `go run ./cmd/securitycadence -json`.
# For every entry that is due_soon or overdue, ensure ONE open issue titled
# "Rotate: <id>" exists (labelled token-rotation). For every open
# token-rotation issue whose entry is back to ok, close it. Needs GH_TOKEN and
# GH_REPO in the environment and `issues: write`. It never touches a secret.
set -euo pipefail

json="${1:?usage: rotation-issues.sh <cadence.json>}"
label="token-rotation"

gh label create "$label" --color BFD4F2 \
  --description "A credential or review in docs/security/security-cadence.md is due" \
  --force >/dev/null

open_titles="$(gh issue list --label "$label" --state open --limit 200 --json number,title)"

# Open (or leave open) an issue for each entry that needs action.
while IFS= read -r row; do
  id="$(jq -r '.id' <<<"$row")"
  title="Rotate: ${id}"
  if jq -e --arg t "$title" 'any(.[]; .title == $t)' <<<"$open_titles" >/dev/null; then
    echo "exists: $title"
    continue
  fi
  status="$(jq -r '.status' <<<"$row")"
  body="$(jq -r '"**\(.obligation)** is \(.status | gsub("_"; " ")) (due \(.due)).\n\nProcedure: \(.action)\n\nWhen done, set `last_done` for `\(.id)` in `docs/security/security-cadence.md` and open a PR; this issue closes itself on the next weekly run once the entry is out of the window."' <<<"$row")"
  gh issue create --title "$title" --label "$label" --body "$body" >/dev/null
  echo "opened: $title ($status)"
done < <(jq -c '.[] | select(.status != "ok")' "$json")

# Close issues whose entry is current again.
while IFS=$'\t' read -r number title; do
  id="${title#Rotate: }"
  if jq -e --arg id "$id" 'any(.[]; .id == $id and .status == "ok")' "$json" >/dev/null; then
    gh issue close "$number" --comment "Register shows \`${id}\` is current again; closing." >/dev/null
    echo "closed: $title"
  fi
done < <(jq -r '.[] | [.number, .title] | @tsv' <<<"$open_titles")
