Add an optional `preferred_name_source` column to contacts. It records how an
operator chose the preferred name (`manual`, `imported`, or `derived`) and is
internal: do not expose it through the REST API or any export format.

Existing rows must be backfilled to `derived`. Add the hand-written up/down
migration, update the GORM model and schema-parity registration, and prove with
a populated fixture that existing contact data survives the migration. Keep the
change append-only.
