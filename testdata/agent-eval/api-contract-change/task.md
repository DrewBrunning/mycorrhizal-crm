Add an optional `preferred_name` string to contact create/update requests and to
the contact response DTO. Update the OpenAPI description and examples, then make
both clients agree with the server.

Regenerate the contract fixtures, generated TypeScript types, and the frozen API
baseline. Add or update route-level tests for the new field. A response that
omits the field when unset is acceptable, but a type mismatch with the committed
contract is not.

Keep the change at the API DTO/client boundary: do not modify the neutral card,
the correspondence layer, or the generated field-compatibility matrix.

This is an API contract change only: do not add a database migration, change the
persisted contact model, or modify import/export or account-bundle services. A
request/response-only field is sufficient.
