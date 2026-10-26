The contact-list `?search=` filter has no measured operation, so an N+1 or
result-size regression in search is invisible to the performance gate.

Add a measured smoke-profile operation for it, register it with an expected
growth class of constant, and make `TestCoreOperationBenchmarks` pass. Regenerate
the performance baseline and report. Do not relax an existing query-count budget
and do not change an unrelated operation.
