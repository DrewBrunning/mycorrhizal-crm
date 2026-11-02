-- Rollback of 000086. Drops the metadata table only; image files under
-- IMAGES_DIR are left for the operator (the release note says so, ADR 0040).
DROP TABLE IF EXISTS activity_images;
