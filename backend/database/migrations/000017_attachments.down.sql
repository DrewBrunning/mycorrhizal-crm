-- Reverses 000017. Destructive by nature: every attachment metadata row is
-- discarded. On-disk files are NOT removed by this migration (it has no
-- knowledge of the attachments directory) — they become orphans.

DROP TABLE attachments;
