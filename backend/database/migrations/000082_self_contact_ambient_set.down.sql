-- Self contact marked ambient (issues #2193, #2192) -- rollback. Drops the
-- provenance flag; contacts.ambient values are untouched, so a "Me" contact that
-- marking made ambient stays ambient (the user can switch it off by hand).
ALTER TABLE users DROP COLUMN self_contact_ambient_set;
