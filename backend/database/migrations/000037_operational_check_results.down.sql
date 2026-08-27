-- Reverses 000037. The table holds only regenerable operational self-check
-- outcomes (no user data), so dropping it loses nothing that the next
-- scheduled integrity check / restore drill will not rewrite.

DROP TABLE IF EXISTS operational_check_results;
