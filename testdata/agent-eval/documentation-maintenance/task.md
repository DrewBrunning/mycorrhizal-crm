Add an operator troubleshooting entry for a failed `genschema` run (for example,
a committed schema fixture that no longer matches the migrated schema) to the
deployment guide. Explain how to identify the drift, the command that
regenerates the fixtures, and what not to do: never hand-edit the dump.

Link the entry from the developer docs and keep every internal link and anchor
valid. Do not change runtime code.

Change only files under `docs/` and `README-developer.md`. Do not edit anything
under `backend/` — even a README there, including the schema-fixture README —
or under `frontend/`, `android/` or `.github/`. The deployment guide and the
developer docs are the only surfaces this task owns.
