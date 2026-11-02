-- Rollback of 000087. Drops the metadata table only; image files under
-- IMAGES_DIR are left for the operator (ADR 0040).
DROP TABLE IF EXISTS note_images;
