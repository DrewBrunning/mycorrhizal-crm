-- The old media scheme stored category='media', key='show'/'movie'/'music'
-- (medium type in Key, no disposition — every row implicitly meant "likes
-- this"). The new scheme moves medium (and, for music/books, facet) into
-- Category and frees Key for disposition (favorite/like/dislike), matching
-- the same category-carries-the-kind convention clothing_size already uses.
-- Backfill existing rows so nothing falls out of the new Media Preferences
-- UI section; every row's only prior implicit meaning was "favorite".
UPDATE preferences SET category = 'media_tv', key = 'favorite' WHERE category = 'media' AND key = 'show';
UPDATE preferences SET category = 'media_movie', key = 'favorite' WHERE category = 'media' AND key = 'movie';
UPDATE preferences SET category = 'media_music_artist', key = 'favorite' WHERE category = 'media' AND key = 'music';

-- Safety net: category/key were never enum-enforced, so a category='media'
-- row with an unrecognized or empty key (shouldn't exist given the old UI
-- only ever wrote show/movie/music, but nothing prevented it) falls back to
-- media_movie/favorite rather than being silently orphaned from the new UI.
UPDATE preferences SET category = 'media_movie', key = 'favorite' WHERE category = 'media' AND key NOT IN ('show', 'movie', 'music');
