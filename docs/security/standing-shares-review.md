# Security review: standing shares (read-only live grants)

The design-time security review that issue #1998's acceptance list requires for
[ADR 0042](../adrs/0042-standing-shares.md). It reviews a design, not code: nothing here is implemented yet, so no
row in [`asvs-l2.md`](asvs-l2.md) changes status in this PR. Section 8 lists the rows the implementing PRs must
update, and section 7 is the test plan those PRs must satisfy before a row can say `satisfied`.

| | |
|---|---|
| **Reviewed** | 2026-10-30, against ADR 0042 as accepted |
| **Issue** | #1998 (supersedes #159) |
| **Standards** | OWASP ASVS 4.0.3 L2 and API Security Top 10 (2023), as pinned in `asvs-l2.md` |
| **Status** | Design review. Re-run as a code review (section 9) when the backend lands. |
| **New trust boundary** | Yes: the first place user A's data is intentionally readable under user B's session, by live lookup rather than by copy. Recorded in `threat-model.md` (Multi-user instances). |

## 1. What is being protected

A grant lets recipient B read an entity owned by A through `GET /share-grants/{id}/content` while the grant is
`active`. The assets at risk are A's contacts, notes, activities and circles, and, second, the *fact* of the
relationship (that A shared with B, and what). The adversaries, all authenticated users of the same instance (the
multi-user model in `threat-model.md`):

- **B**, a legitimate recipient, trying to read beyond the grant (more sections, sensitive items, other entities,
  other users' grants).
- **C**, a third user, trying to read, enumerate or probe grants and entities that are not theirs.
- **A as sender**, probing whether B blocked them or whether B read the data.
- **A compromised session** of either party.

Out of scope: B copying data they were legitimately shown (ADR 0042 decision 10 states this is not recallable), and
the operator reading the database (the self-hosted boundary).

## 2. Inventory of new read paths

Every path below is a read path for ADR 0042 and gets the ownership and sensitivity tests in section 7. A path not
in this table must not exist; the route census (section 7.1) makes an addition fail CI.

| ID | Surface | Who | Data returned | Enforcement point |
|---|---|---|---|---|
| R1 | `GET /share-grants/{id}/content`, contact | recipient | filtered neutral Card (identity + granted sections) | grant resolver, then `models.RecordForContactFiltered` + `models.ApplyFieldSelection`, `IncludeSensitive=false` |
| R2 | same route, note | recipient | note text and date | grant resolver, then allowlist view |
| R3 | same route, activity | recipient | title, description, location, dates, type | grant resolver, then allowlist view |
| R4 | same route, circle | recipient | circle metadata and live member display names | grant resolver, then allowlist view, member query scoped by owner |
| R5 | `GET /share-grants/incoming` | recipient | pending/active rows: sender username, type, per-request label | `to_user_id` scope in the query |
| R6 | `GET /share-grants/outgoing` | owner | own rows, status, expiry, section list, live member count | `from_user_id` scope in the query |
| R7 | pending-grant label (inside R5) | recipient, pre-acceptance | display name / circle name / activity title | resolver + owner-scoped lookup |
| R8 | audit events for grant transitions | each party, own trail | grant id and fields, no entity content | recorder, own `user_id` |
| R9 | notification payload (in-app, then push/email) | recipient | sender username + entity type only | notifier |
| R10 | trash restore response | owner | ids/types of grants revoked by the delete | owner-scoped |
| R11 | Share dialog preview | owner | the same projection the recipient would get | resolver in owner-preview mode, scoped to `from_user_id` |

Writes (create, patch, accept, decline, leave, revoke, reconfirm, block) are covered by the same ownership rule and
the lifecycle tests, but they return grant metadata only, never entity content.

## 3. Authorization model at the data-access boundary

**Rule.** Every R1-R4 read is authorized by *both* the grant and the authenticated recipient, and the source entity
is then fetched scoped by the grant's *owner*. There are two scoped lookups, never one unscoped lookup:

1. Resolve the grant with a single statement that carries every condition:
   `id = ? AND to_user_id = <session user> AND status = 'active' AND (expires_at IS NULL OR expires_at > <injected now>)`.
   No row means `404 share_grant_not_found`, whatever the reason.
2. Fetch the entity with the owner from the *grant row*, not from the request:
   `user_id = grant.from_user_id` and the entity's own live condition (`deleted_at IS NULL`, not archived;
   contact additionally `vcard_uid = grant.entity_id`). No row means the same `404`.
3. Steps 1 and 2 run inside one read transaction so they see one snapshot (section 6).

**One chokepoint.** The resolver is a single service function in the service layer (ADR 0035). Controllers cannot load
an entity "for a grant" any other way: the entity loaders used by grant reads take a `GrantContext` value that only
the resolver can construct. A source-level test (section 7.2) fails if any other function in the controller package
reads `share_grants`, or if a grant view type is built from a model outside the resolver's callers.

**Never trust request-supplied identity.** `entity_id`, `to_user_id` and `from_user_id` on a read come from the grant
row. The request supplies only the grant id. `POST /share-grants` is the only place a client names an entity, and
there the entity is resolved with `user_id = <session user>` (contact: `vcard_uid` and `user_id`), so a foreign entity
is `404`, identical to a nonexistent one. This is the same ownership rule as
`backend/controllers/circle_controller.go#GetCircle`.

**Role separation.** Owner routes filter on `from_user_id`, recipient routes on `to_user_id`. Calling a route in the
wrong role is `404`, not `403`. An owner cannot read their own grant's `content`; they use the preview (R11), which is a
separate code path with an explicit mode, so the recipient route has no "owner bypass" branch to get wrong.

**Admin.** `is_admin` grants nothing here. Admins cannot read other users' grants or content, consistent with
`threat-model.md` (the admin role administers accounts, it does not read content).

**Credentials.** The routes sit in the `protected` group and require session or `api:read`/`api:write` scope.
CardDAV/CalDAV app passwords, feed tokens, MCP and webhook paths do not reach them, and granted data is excluded from
those surfaces (ADR 0042 decision 14).

## 4. Sensitivity enforcement point

Sensitivity is enforced **in the query / projection, on every read, independent of the grant**:

- Contact (R1): `RecordForContactFiltered` with `IncludeSensitive=false` applies the normal-only filter to
  relationship edges, hobby/preference rows and vCard custom fields at query time, and `ApplyFieldSelection` removes
  private/secret addresses with their coordinates. The grant row has no field that can turn this off, so no request
  can. A test that sets `sensitivity` to every non-normal value on every sensitivity-bearing item type asserts absence
  in the response body and in every serialization the route can emit.
- Note, activity, circle (R2-R4): these types have no sensitivity attribute today. They are protected by being served
  from allowlist view types, not model structs, so an added column (including a future sensitivity column) is not
  exposed until a person adds it. A reflection-based **field census** lists every exported field of `Note`,
  `Activity`, `Circle` and the contact flat columns, requires each to be classed `exposed` or `withheld` with a reason,
  and fails when a field is unclassified.
- Reclassification: moving an item to private/secret removes it on the next read (the query filter), and adds the
  section to `reconfirm_sections` (ADR 0042 decision 5), withholding the section until the owner re-confirms. Moving
  it back to normal does not re-expose it. The marker is set at the persistence point for each sensitivity-bearing
  item (address, relationship edge, preference, field value), not in controllers, and a census test enumerates every
  writer of those columns (REST, import, CardDAV pull, merge, bundle restore) and requires each to go through it.
- Not an access boundary: #1997's per-contact field visibility. A grant read neither consults nor trusts it
  (ADR 0042 decision 14). A test hides a field via #1997 and asserts the grant projection is unchanged.
- `include_sensitive` is absent from the route and the DTO. A test posts it as a query parameter and a body field and
  asserts it is ignored, and a binding test asserts `POST`/`PATCH` DTOs contain no such field.

## 5. Enumeration and existence oracles

| Probe | Required behaviour |
|---|---|
| `GET /share-grants/{random uuid}/content` vs a real grant that is not yours | identical `404`, body, headers and timing class |
| content of a pending, declined, revoked, expired or blocked grant | same `404` as above |
| content when the entity is deleted, trashed, archived or the owner deleted | same `404` |
| owner calls the recipient route; recipient calls an owner route | same `404` |
| `POST /share-grants` with another user's entity id | `404`, identical to a nonexistent id; no row created |
| `POST /share-grants` to a user who blocked you | `201` as `pending` from your side; no recipient-visible row, no notification (the sender cannot learn the block) |
| `POST /share-grants` to a nonexistent `to_user_id` | `404`; this discloses nothing beyond P13, which already lists every id |
| `GET /share-grants/incoming` | only your own rows; no count, cursor or `total` that includes anyone else's |
| duplicate create of a live grant | `409` to its sender; a `blocked` row counts as the sender's own pending grant, so the response is the same as for an unblocked pending duplicate |
| error bodies | no entity name, section list, owner or recipient id of a grant the caller is not party to |
| pending label | computed only for the recipient of that grant; an unaccepted grant never discloses content beyond the label |
| audit and notifications | carry grant id, sender username and entity type only, never labels or content (R8, R9) |
| response timing | the resolver makes one grant lookup, then one entity lookup only on success; no per-reason branch changes the response, so no measured timing test is required |

Ids are UUIDs (not sequential), so id guessing is not a practical oracle, but the table does not rely on that.

## 6. Revocation, expiry and races

- **Expiry is evaluated at read time** with the injected clock inside the resolver's `WHERE`, so a stalled or failed
  expiry job cannot extend access. The job only reconciles `status`, audit and notification.
- **Revoke vs in-flight read.** SQLite serializes writers, and a read transaction sees a single snapshot. A read whose
  snapshot precedes the revoke commit returns data that was legitimately authorized when the snapshot was taken; any
  read starting after the commit gets `404`. There is no window in which a read after a *committed* revoke succeeds.
  This is equivalent to "the recipient read it a moment earlier" and is documented as the recall limit.
- **No caching across a revoke.** Grant responses are `Cache-Control: no-store`, carry no ETag and honour no
  `If-None-Match`; the service worker must not cache `/share-grants/*/content` (frontend trap on SW caching; add to the
  denylist and test it). `ASVS 8.1.1/8.2.1` already require no-store on `/api/`; the test asserts it for this route.
- **Accept vs revoke race.** Transitions are conditional updates (`UPDATE ... WHERE id = ? AND status = 'pending'`, rows
  affected checked), per the atomic CAS pattern of ADR 0018. Two concurrent transitions yield one winner and one `409`;
  a revoke can never be overwritten by a later accept.
- **Delete vs read.** The entity delete path revokes in the same transaction as the delete; the resolver also requires
  the live source row, so a code path that forgets the revoke fails closed.
- **Reclassification vs read.** The marker write and the sensitivity write share one transaction, so there is no state
  where an item is non-normal but the section is not yet withheld. The query filter covers that window regardless.
- **User deletion mid-flight.** `deleteUserCascade` runs in one transaction; a read after commit finds no grant.
- **Trash/restore.** Restore does not re-arm. A test restores a contact and asserts the recipient still gets `404`.

## 7. Test plan (to be satisfied by the implementing PRs)

Test-first per the repo workflow. Tests use `dbtest.New` (the real migrated schema), the injected clock and the
six-persona matrix helpers. Citations in the ASVS rows must be real test names, so names below are the contract.

### 7.1 Route census and authorization matrix

- Every new route appears in `backend/routes/authorization_matrix_test.go` with its class and in
  `backend/routes/authorization_matrix_mutations_test.go` for the writes; `backend/cmd/bolacheck` must pass.
- The outward census (`backend/models/contact_record_outward_census_test.go`) lists R1 as a deliberate cross-user
  surface with `IncludeSensitive=false`, and a test fails if any other surface reads `share_grants`.
- No grant route is reachable with a CardDAV/CalDAV app password, a feed token or the MCP scope.

### 7.2 Ownership regression for every read path (R1-R11)

For each of R1-R4, R5-R7 and R11, with personas owner (A), recipient (B), unrelated user (C), admin and anonymous:

- `Test<Path>_RecipientReadsGrantedData`: B gets exactly the projection.
- `Test<Path>_OtherUserGets404`: C gets `404`; the body equals the not-found body byte for byte.
- `Test<Path>_OwnerCannotUseRecipientRoute` and `..._RecipientCannotUseOwnerRoute`: `404`.
- `Test<Path>_AdminGets404`.
- `Test<Path>_GrantForForeignEntityRejected`: A names C's entity at create: `404`, no row.
- `Test<Path>_EntityIDComesFromGrant`: a request that tries to substitute another `entity_id` or owner (query, body,
  header) is ignored or `400`.
- `TestGrantReadsOnlyThroughResolver`: a source scan fails when any code outside the resolver reads `share_grants` or
  builds a grant view from a model.
- Cross-owner: A and C both own an entity with the same `vcard_uid`/UUID value (possible after a bundle restore); a
  grant from A never resolves C's row.

### 7.3 Sensitivity regression for every read path

- `TestGrantContent_ExcludesNonNormal_<ItemType>`: for each of address (with coordinates), relationship edge,
  preference, vCard custom field, and every other item type in a sensitivity-bearing section, values `private` and
  `secret` never appear in the body.
- `TestGrantContent_IncludeSensitiveIgnored`: query and body forms.
- `TestGrantContent_FieldVisibilityIsCosmetic` (#1997).
- `TestGrantViews_FieldCensus` for `Note`, `Activity`, `Circle` and contact columns (section 4).
- `TestGrantCircle_MembersAreNamesOnly` and `..._ExcludesArchivedAndDeletedMembers`.
- `TestGrantActivity_OmitsParticipantsAndExternalRef`, `TestGrantNote_OmitsContactLink`.
- The existing sensitivity census tests (`contact_record_outward_census_test.go`) gain the grant surface.

### 7.4 Field selection and re-confirmation

- `TestGrantContent_SectionsReappliedEveryRead`: edit the contact between two reads; a section not in `sections` stays
  absent, and a new item in a granted section appears.
- `TestGrantContent_UnknownStoredSectionIgnored`: a section token removed in a later release narrows, never widens.
- `TestGrantContent_NewSectionNeverExposed`: a section added to `FieldSections` after grant creation is absent.
- `TestGrantCreate_EmptySectionsRejected`, `..._UnknownSectionRejected`, `..._SectionsOnNonContactRejected`.
- `TestReconfirm_NormalToPrivateWithholdsSection`: marker set, whole section absent, other sections unchanged.
- `TestReconfirm_BackToNormalStaysWithheldUntilReconfirm`.
- `TestReconfirm_OwnerOnly_Audited` and `..._ClearsMarkerAndReexposes`.
- `TestReconfirm_AllWriters`: REST, import, CardDAV pull, merge and bundle restore each set the marker when they flip
  an item to non-normal (the census over writers, section 4).
- `TestGrantPatch_WideningRejected` (create a new grant instead) and `..._NarrowingTakesEffectNextRead`.

### 7.5 Revocation, expiry and entity deletion: no lingering access

Each asserts `404` on the next request and, where relevant, the audit row and notification:

- `TestRevoke_NextReadIs404` for each entity type; `TestRevoke_IdempotentAndTerminal`; `TestRevoke_ByNonOwner404`.
- `TestExpiry_EffectiveWithoutJob`: set the clock past `expires_at`, do not run the job, assert `404`; then run the job
  and assert `expired`, audit and notification, and that the job is idempotent and catches up after downtime (ADR 0011).
- `TestPendingTTL_Expires` and `TestPendingGrant_ExposesNothing` (content is `404` until accepted).
- `TestEntityDeleted_RevokesGrant_<contact|note|activity|circle>` including bulk delete and merge loser.
- `TestContactTrash_RevokesGrants_RestoreDoesNotRearm` (ADR 0037, ADR 0030 decision 6).
- `TestCircleMembership_LiveBothWays`: add and remove a member between reads.
- `TestRecreatedContact_DoesNotInheritGrant` (same `vcard_uid` after trash, as in ADR 0037).
- `TestSourceUserDeleted_GrantsGone` and `TestRecipientUserDeleted_OwnerAuditedAndRowGone` for admin `DeleteUser` and
  self-service `DeleteOwnAccount`; the existing delete-cascade structural test must list the new tables.
- `TestBlock_EndsGrants_SenderSeesPending_NoNotification`, `TestBlocked_CreateIndistinguishable`.
- `TestAcceptRevokeRace` and `TestExpireRevokeRace` using the conditional-update pattern.
- `TestRetention_TerminalRowsPurged_ActiveKept`, `TestPurgeCompleteness` extended to `share_grants` and `share_blocks`.
- `TestNoCachingHeaders_GrantContent`.

### 7.6 Migration and model tests

Hand-written up/down pair; `dbtest.New`; `schema_parity_test.go` registration; the CHECK-constraint tests (status,
entity type, from <> to, revoked reason); the partial unique index test (a live duplicate fails, a history row does
not); the symmetric down migration; the audit-CHECK rebuild preserving existing audit rows and the hash chain
(`audit-verify`); a populated-database upgrade check; the updated schema dump via `genschema`.

### 7.7 Break-the-code checks

Per the workflow, mutate the implementation and confirm a test fails for each: drop `to_user_id` from the resolver
`WHERE`; drop the `expires_at` condition; drop the `status` condition; take `user_id` from the request instead of the
grant; set `IncludeSensitive=true`; skip the marker in one writer; skip the revoke in one delete path; return `403`
for a wrong-role call.

## 8. ASVS L2 and API Top 10 mapping

Rows the implementing PRs must update with real citations (no row is changed by this design PR):

| Control | Why it applies | Design response |
|---|---|---|
| 1.4.1, 1.4.4, 1.4.5 | trusted enforcement, single mechanism, attribute-based | one resolver (section 3); grant is the attribute; no second mechanism |
| 4.1.1 | access control on a trusted layer | server-side resolver and scoped queries |
| 4.1.2 | policy data not user-manipulable | status, owner and timestamps are server-set; DTO allowlists |
| 4.1.3 | least privilege; the audit of cross-user surfaces in that row | adds one deliberate cross-user surface (R1-R4), read-only and normal-only |
| 4.1.5 | fail securely | all denial is the same `404`; missing live source fails closed |
| 4.2.1 | IDOR protection on create/read/update/delete | section 7.2 per-path tests |
| 4.2.2 | anti-CSRF | no state-changing `GET`; reads are `GET`, transitions are `POST`/`PATCH` with the existing CSRF mitigations |
| 5.1.2, 5.1.3 | mass assignment, allow-list validation | create/patch DTOs bind five/two fields; section tokens validated against `FieldSections` |
| 7.2.1 / 7.3.3 | security-relevant events logged; trail protected | grant transitions land in the hash-chained audit trail, no content in snapshots |
| 8.1.1, 8.2.1 | no caching of sensitive data | `no-store`, no ETag, service worker exclusion |
| 8.3.1 | no sensitive data in URLs | grant id only in the path; sections in the body |
| 8.3.2, 8.3.4 | data lifecycle, sensitivity policy | retention variable, deletion cascade, normal-only policy; `data-retention-lifecycle.md` and `pii-inventory.md` entries |
| 11.1.4 | anti-automation | caps on pending grants per sender and per pair; standard API rate limiting on reads |
| 13.1.4, 13.2.1, 13.2.2 | authz at URI and resource level, methods, schema | route census, only implemented methods, `ValidateJSONMiddleware` |
| API1 (BOLA) | object-level authorization | section 3 |
| API3 (property-level authorization) | which fields leave | allowlist views, sections, sensitivity filter |
| API4 (resource consumption) | spam grants, large circles | caps, circle member cap in the response |
| API5 (function-level authorization) | owner vs recipient functions | role-scoped routes, wrong role is `404` |
| API6 (sensitive business flows) | granting access is a sensitive flow | acceptance, audit, notification, block |

`asvs-l2.md` P13 gets a sentence in the implementing PR noting that the directory now also feeds the grant picker, and
`threat-model.md` Actors x trust boundaries gets a row for "Recipient or third user probing shared data". This PR adds
the pointer in `threat-model.md` (Multi-user instances) only.

## 9. Residual risks and review follow-ups

- **Recall.** Data a recipient has seen cannot be recalled. Accepted and disclosed (ADR 0042 decision 10).
- **Live circle membership** discloses newly added members' names without a prompt (ADR 0042 open question 1).
- **Whole-section re-confirmation** withholds more than the reclassified item (open question 3). Fail-closed.
- **Directory exposure** (P13) means any user can be named as a recipient; acceptance, blocking and caps bound the harm.
- **Notification channels** are third parties; payloads carry no content. The general send path is its own review item.
- **Attachments and images** are deliberately out of v1; adding them is a new review (path traversal, MIME, EXIF
  stripping per P10 apply to a recipient read route).
- **Re-review gates.** Repeat this review as a code review when the backend PR lands, record the result as a
  changelog row in `asvs-l2-verification-report.md`, and re-run it before any of: editable grants, group/household
  grants, `include_sensitive` for grants, attachment sharing, or a restricted-directory change (#2218).
