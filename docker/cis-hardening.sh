#!/usr/bin/env bash
#
# CIS-aligned container hardening check (issue #362).
#
# A short, deterministic pass/fail baseline against the *built* all-in-one
# image, mapped to the CIS Docker Benchmark controls that matter for this
# artifact. It deliberately does not shell out to docker-bench-security: that
# tool's image checks are host/daemon-oriented, need --privileged + host mounts
# (fragile in GH runners), and its 4.8 "setuid/setgid removed" check is a NOTE
# no-op that never inspects the filesystem. Everything here is done with
# `docker inspect` / `docker create` + `docker export` + `tar`, which are
# already on the runner.
#
# Usage:
#   docker/cis-hardening.sh [IMAGE] [EXCEPTIONS]
#
#   IMAGE       image reference to inspect (default mycorrhizal-crm-test:latest)
#   EXCEPTIONS  accepted-exceptions file (default <script dir>/cis-hardening.ignore)
#
# Each check emits one line: `[PASS]` / `[FAIL]` / `[WARN]` / `[ACCEPT]` with a
# CIS id and description. A non-PASS result (FAIL or WARN) fails the run unless
# a matching `<id> <status>` line appears in the exceptions file — recorded
# acceptances with justification, in the spirit of android/.mobsf's ignore-list.
# Exit status is 0 when every check passes or is accepted, 1 otherwise.
set -uo pipefail

IMAGE="${1:-mycorrhizal-crm-test:latest}"
EXCEPTIONS="${2:-$(dirname "$0")/cis-hardening.ignore}"

checked=0
failed=()

# is_excepted <id> <status>  — true if the exceptions file accepts that result.
is_excepted() {
  local id="$1" status="$2" line id_match rest status_match
  [ -f "$EXCEPTIONS" ] || return 1
  while IFS= read -r line; do
    line="${line%%#*}"                        # strip trailing comment
    [ -n "$(printf '%s' "$line" | tr -d '[:space:]')" ] || continue
    id_match="${line%%[[:space:]]*}"
    rest="${line#"$id_match"}"
    rest="${rest#"${rest%%[![:space:]]*}"}"   # strip leading whitespace
    status_match="${rest%%[[:space:]]*}"
    if [ "$id_match" = "$id" ] && [ "$status_match" = "$status" ]; then
      return 0
    fi
  done < "$EXCEPTIONS"
  return 1
}

# emit <status> <id> <desc> [detail]
emit() {
  local status="$1" id="$2" desc="$3" detail="${4:-}"
  checked=$((checked + 1))
  if [ "$status" = "PASS" ]; then
    printf '[PASS] %s - %s\n' "$id" "$desc"
    return
  fi
  if is_excepted "$id" "$status"; then
    printf '[ACCEPT] %s - %s (%s)\n' "$id" "$desc" "${detail:-accepted exception}"
    return
  fi
  printf '[%s] %s - %s %s\n' "$status" "$id" "$desc" "$detail"
  failed+=("$id")
}

# --- image config checks (no container started) ------------------------------

# 4.1 — Ensure that a user for the container has been created (non-root).
user=$(docker inspect --format '{{.Config.User}}' "$IMAGE" 2>/dev/null || true)
case "$user" in
  ""|"<no value>")
    emit WARN 4.1 "Ensure a non-root container user" \
      "no USER instruction — runs as root at PID 1 by design (entrypoint drops the backend to appuser)"
    ;;
  "root"|"0")
    emit FAIL 4.1 "Ensure a non-root container user" \
      "image declares USER root/0"
    ;;
  *)
    emit PASS 4.1 "Ensure a non-root container user" "USER=$user"
    ;;
esac

# 4.6 — Ensure HEALTHCHECK instructions have been added.
healthcheck=$(docker inspect --format '{{.Config.Healthcheck}}' "$IMAGE" 2>/dev/null || true)
case "$healthcheck" in
  ""|"<nil>"|"<no value>")
    emit FAIL 4.6 "Ensure HEALTHCHECK is present" "no HEALTHCHECK instruction"
    ;;
  *)
    emit PASS 4.6 "Ensure HEALTHCHECK is present" ""
    ;;
esac

# --- filesystem checks (docker create + export, no app start) ----------------

tar_listing=""
cid=$(docker create "$IMAGE" 2>/dev/null || true)
if [ -n "$cid" ]; then
  tarfile=$(mktemp)
  if docker export "$cid" > "$tarfile" 2>/dev/null; then
    tar_listing=$(tar -tvf "$tarfile" 2>/dev/null || true)
  fi
  rm -f "$tarfile"
  docker rm "$cid" >/dev/null 2>&1 || true
fi

if [ -z "$tar_listing" ]; then
  emit FAIL 4.8 "No setuid/setgid binaries" "could not inspect image filesystem"
  emit FAIL x1 "Writable dirs owned only by appuser" "could not inspect image filesystem"
else
  # 4.8 — Ensure setuid and setgid permissions are removed (files only; a setgid
  # home directory is not a binary and is not flagged).
  setid=$(printf '%s\n' "$tar_listing" | awk '$1 ~ /^-/ && $1 ~ /[sS]/ {print $1 " " $6}')
  if [ -z "$setid" ]; then
    emit PASS 4.8 "No setuid/setgid binaries" ""
  else
    emit FAIL 4.8 "No setuid/setgid binaries" "found: $(printf '%s' "$setid" | tr '\n' ' ')"
  fi

  # x1 — writable runtime dirs are owned by appuser and not group/world-writable.
  # Owner UID != 0 and no `w` in the group or other positions of the mode.
  bad_dirs=""
  for path in app/data app/static/photos app/static/attachments; do
    entry=$(printf '%s\n' "$tar_listing" | awk -v p="$path/" '$6 == p {print $1, $2}')
    if [ -z "$entry" ]; then
      bad_dirs="$bad_dirs $path(missing)"
      continue
    fi
    mode=$(printf '%s' "$entry" | awk '{print $1}')
    uid=$(printf '%s' "$entry" | awk '{print $2}' | cut -d/ -f1)
    group_w="$(printf '%s' "$mode" | cut -c6)"
    other_w="$(printf '%s' "$mode" | cut -c9)"
    if [ "$uid" = "0" ] || [ "$group_w" = "w" ] || [ "$other_w" = "w" ]; then
      bad_dirs="$bad_dirs $path($mode,$uid)"
    fi
  done
  if [ -z "$bad_dirs" ]; then
    emit PASS x1 "Writable dirs owned only by appuser" ""
  else
    emit FAIL x1 "Writable dirs owned only by appuser" "bad:$bad_dirs"
  fi
fi

# --- runtime config check (shipped compose files) ----------------------------

# 5.3/5.4 — the shipped compose files must not run privileged or add
# capabilities; supervisord already runs as root to drop the backend, so any
# extra privilege would be escalation surface with no consumer.
compose_bad=""
for f in docker-compose.yml docker-compose.test.yml; do
  if [ -f "$f" ] && (grep -Eq '^[[:space:]]*privileged:[[:space:]]*true' "$f" || grep -Eq '^[[:space:]]*cap_add:' "$f"); then
    compose_bad="$compose_bad $f"
  fi
done
if [ -z "$compose_bad" ]; then
  emit PASS 5.3 "No privileged / cap_add in compose" ""
else
  emit FAIL 5.3 "No privileged / cap_add in compose" "found in:$compose_bad"
fi

# --- summary -----------------------------------------------------------------

if [ "${#failed[@]}" -gt 0 ]; then
  echo "::error::CIS hardening baseline failed: $(IFS=,; echo "${failed[*]}")"
  exit 1
fi
echo "OK: $checked CIS hardening checks passed or accepted ($IMAGE)."
