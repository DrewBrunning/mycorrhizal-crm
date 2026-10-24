Add an optional `preferred_name` string to contact create/update requests and to
the contact response DTO. Update the OpenAPI description and examples, then make
both clients agree with the server.

Regenerate the contract fixtures, generated TypeScript types, and the frozen API
baseline. Add or update route-level tests for the new field. A response that
omits the field when unset is acceptable, but a type mismatch with the committed
contract is not.
