Add an optional `nickname` field to a contact and carry it through the whole
contact lifecycle: the neutral card, the flat persisted columns, vCard 3.0 and
4.0, JSContact, and the CSV export.

A nickname set through any input must survive a round trip through each format
and must not disappear when an unrelated field is edited. Update the
correspondence oracle and regenerate the field-compatibility matrix; update the
generated web types if the API shape changes.
