-- Reverses 000030's three deterministic branches. Not a perfect inverse: rows
-- that landed on media_movie/favorite via the up migration's catch-all branch
-- (unrecognized/empty legacy key) become indistinguishable from rows that
-- were genuinely media_movie/favorite from the start, so both are pulled back
-- to category='media', key='movie' below. Acceptable for a same-release
-- migration pair's rollback, not a design meant for repeated up/down cycling
-- (see 000012's down migration for the same class of documented tradeoff).
UPDATE preferences SET category = 'media', key = 'show' WHERE category = 'media_tv' AND key = 'favorite';
UPDATE preferences SET category = 'media', key = 'movie' WHERE category = 'media_movie' AND key = 'favorite';
UPDATE preferences SET category = 'media', key = 'music' WHERE category = 'media_music_artist' AND key = 'favorite';
