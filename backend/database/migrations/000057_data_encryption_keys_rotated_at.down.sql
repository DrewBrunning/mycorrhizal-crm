-- Rollback of 000057 (issue #955). A downgrade also reverts the binary to code
-- that does not surface key age, so the rotation timestamp has no reader. It is
-- derived metadata about a key the DEK row already identifies, not user content
-- -- dropping it loses nothing recoverable. created_at remains.
ALTER TABLE data_encryption_keys DROP COLUMN rotated_at;
