-- Rollback of 000088. Drops the gallery table and the main-image pointer.
-- Image files under IMAGES_DIR are left for the operator (ADR 0040).
DROP INDEX IF EXISTS idx_contacts_main_image_id;
ALTER TABLE contacts DROP COLUMN main_image_id;
DROP TABLE IF EXISTS contact_images;
