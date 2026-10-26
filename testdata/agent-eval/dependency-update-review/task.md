A transitive npm package has an unfixed advisory (CVE-2026-9999) that cannot be
upgraded yet because the parent package pins it.

Record a time-bounded exception in the dependency-advisory ledger with the
affected package, a concrete reason, owner `drew`, and an expiry inside the
policy's 90-day window from today. Do not upgrade the package and do not swallow
the finding in a scanner ignore list. Run the exception gate, and regenerate the
notices only if the dependency metadata actually changes.
