Add an operator troubleshooting entry for a failed `genschema` run (for example,
a committed schema fixture that no longer matches the migrated schema) to the
deployment guide. Explain how to identify the drift, the command that
regenerates the fixtures, and what not to do: never hand-edit the dump.

Link the entry from the developer docs and keep every internal link and anchor
valid. Do not change runtime code.
