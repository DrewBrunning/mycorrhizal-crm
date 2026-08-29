-- Import source links (issues #351 + #353) — rollback.
--
-- The ledger is derived bookkeeping: every row it describes is real data the
-- importer created, and the links themselves are rebuildable by re-running the
-- import (which is exactly what they gate). Dropping the table loses only the
-- idempotency memory, not user data — but it does mean a re-run after the
-- downgrade would duplicate, so this rollback should not be taken casually.
DROP TABLE import_source_links;
