Add a weekly scheduled workflow that runs the dependency-exception freshness
gate and the agent-context validator against the trusted main tree.

Register it for nightly failure alerting, including the reconcile list, and
document it in the release-gate docs. Do not add a `pull_request` or
`pull_request_target` trigger and do not make it a required per-PR check.
