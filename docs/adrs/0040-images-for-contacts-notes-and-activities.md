# ADR 0040: Images for contacts, notes and activities

- **Status:** accepted
- **Date:** 2026-10-30
- **Decides:** issue #2004 (the ADR its acceptance list requires) and the model question in #1994.
- **Related:** `docs/adrs/0004-soft-vs-hard-delete-semantics.md`, `docs/adrs/0015-temporal-semantics.md`,
  `docs/adrs/0035-service-boundary-and-delete-cascade-registry.md`, `docs/adrs/0037-contact-trash-and-restore.md`,
  `docs/adrs/0038-error-reason-codes.md`, `docs/adrs/0028-local-only-android-mode-and-server-profiles.md`
  (account bundle), backend/AGENTS.md traps #5-#7.

## Context

A contact has one profile picture (`contacts.photo` plus `PhotoThumbnail`, written by
`backend/controllers/photo_controller.go#AddPhotoToContact`) and contact-scoped file attachments
(`backend/models/attachment.go#Attachment`). A note is text only and an activity cannot carry an image.
#1994 proposed a separate `ActivityAttachment`; #2004 proposed one `images` table with nullable
`contact_id` / `note_id` / `activity_id`. This ADR picks the shape once, so activity images (#1994), note
images and the contact gallery are built from the same parts instead of three storage paths.

Facts in the code that the design depends on:

1. **No library exists for orientation or capture time.** There is no EXIF reader in `go.mod` and no
   `golang.org/x/image`. `backend/attachments/exif_strip.go#StripImageMetadata` only *removes* metadata.
   The avatar path decodes JPEG, PNG and HEIC (`github.com/gen2brain/heic`, a WASM decoder run by
   `wazero`) and resizes with `github.com/nfnt/resize`.
2. **Sensitivity is not a column on contacts, notes or activities.** It exists on relationship edges,
   field definitions and values, preferences, addresses and occasions. "Inherit the parent's sensitivity"
   therefore has no stored input today; decision 7 makes it a single resolver so a future per-parent
   sensitivity needs no image migration.
3. **Contact delete does not delete activities.** `ContactCascadeRegistry` removes only the
   `activity_contacts` join row (`backend/services/contact_delete.go#ContactCascadeRegistry`), because an
   activity survives its other attendees. Activity images are owned by the activity, not the contact.
4. **Trash restore is driven by the snapshot's `Soft` id map, per table name**
   (`backend/services/contact_trash.go#ContactDeletionPayload`), so any `CascadeSoft` registry step is
   restorable with no per-table restore code.
5. **No file sweeper exists.** ADR 0037 decision 5 leaves a failed post-commit file removal "for the orphan
   sweep"; the only file removal today is the post-commit `removePurgedFiles` in
   `backend/services/purge_service.go`.

## Decision

### 1. Model: per-owner tables built by composition, not one polymorphic table

Three tables, one per owner kind, each with a real `NOT NULL` owner key:

| Table | Owner key | Created in slice |
|---|---|---|
| `activity_images` | `activity_id` -> `activities.id` | (a) |
| `note_images` | `note_id` -> `notes.id` | (b) |
| `contact_images` | `contact_vcard_uid` (the same key `Attachment` uses) | (c) |

Rejected: one `images` table with three nullable owner columns. It needs a CHECK "exactly one owner is
set" that no cascade test can see, a nullable FK on every row, a three-way join for every ownership
check, and one shared blast radius for a bug in any owner's delete path. Per-owner tables give each owner
a `NOT NULL` key, its own indexes, its own cascade registration and its own row cap, and the completeness
tests (`backend/controllers/delete_cascade_coverage_test.go`) can see each table. They are not three
implementations: everything except the owner column is shared by composition.

**Shared row set**, one embedded struct, new in the `models` package:

```go
// ImageFields is every column an image row has except its owner key.
type ImageFields struct {
    gorm.Model                                                   // soft delete (decision 4)
    UserID uint   `gorm:"not null;index" json:"-"`               // every query is scoped by it (trap #5)
    UUID   string `gorm:"column:uuid;not null;uniqueIndex" json:"uuid"` // bundle id and ETag seed

    StoredName     string `gorm:"column:stored_name;not null" json:"-"` // server-generated, never user input
    ThumbName      string `gorm:"column:thumb_name;not null" json:"-"`
    MIMEType       string `gorm:"column:mime_type;not null" json:"mime_type"`   // image/jpeg | image/png, per row
    ThumbMIMEType  string `gorm:"column:thumb_mime_type;not null" json:"-"`
    Width, Height  int    `gorm:"not null" json:"width"`                          // of the stored full variant
    SizeBytes      int64  `gorm:"column:size_bytes;not null" json:"size_bytes"`
    ThumbSizeBytes int64  `gorm:"column:thumb_size_bytes;not null" json:"-"`

    DHash     *int64     `gorm:"column:dhash" json:"-"`         // 64-bit difference hash (decision 9)
    TakenAt   *time.Time `gorm:"column:taken_at" json:"taken_at"`   // instant, UTC (ADR 0015 category 1)
    TakenDate *string    `gorm:"column:taken_date" json:"taken_date"` // YYYY-MM-DD wall-clock day (category 2)
    CropX, CropY, CropW, CropH *float64                           // optional crop, fractions of the full variant
}
```

The three models differ only by the owner field:

```go
type ActivityImage struct { ImageFields; ActivityID uint `gorm:"column:activity_id;not null;index"` }
type NoteImage     struct { ImageFields; NoteID     uint `gorm:"column:note_id;not null;index"` }
type ContactImage  struct { ImageFields; ContactVCardUID string `gorm:"column:contact_vcard_uid;not null;index"` }
```

All three are registered in `backend/models/schema_parity_test.go`. The original filename is **not**
stored: it is PII-adjacent, display-only, and the re-encode discards it anyway. `taken_date` is the
camera's local wall-clock day exactly as written in EXIF. `taken_at` is set only when the EXIF carries a
zone offset; a zone-less capture time is kept as the day only, because ADR 0015 rule 7 would label it UTC
and can shift the day.

**Shared behaviour**, written once (a new `backend/imaging` package and a new image service in the `services` package):

- **Storage service.** `imaging.Store{Dir}` writes `<IMAGES_DIR>/u<user_id>/<uuid>.<ext>` and
  `<uuid>_t.<ext>`. Names come from `uuid.New()`; the only other path input is the numeric authenticated
  `user_id`, and `imaging.StoredPath` re-validates both the way
  `backend/attachments/attachments.go#StoredPath` does (bare-name equality, no `..`). Files are written to
  `<name>.tmp` and renamed, so a reader never sees a partial file. `IMAGES_DIR` is a new absolute-path
  setting defaulting beside `PROFILE_PHOTO_DIR`, as `ATTACHMENTS_DIR` does.
- **Pipeline.** `imaging.Process(r io.Reader, lim Limits) (*Result, error)` is the only decoder and encoder
  (decision 2) and knows nothing about owners.
- **Generic service.** One repository/service, parameterized by owner kind, carries upload, list, fetch,
  delete, adopt and count:

```go
// ImageRow is implemented, by pointer, by the three models.
type ImageRow interface {
    Fields() *ImageFields
    OwnerColumn() string // "activity_id" | "note_id" | "contact_vcard_uid"
    SetOwner(OwnerRef)
}

type ImageService[T any, P interface{ *T; ImageRow }] struct {
    DB    *gorm.DB
    Store *imaging.Store
    Kind  OwnerKind // limits, metrics and audit labels only
    Limit int       // max live images per owner (decision 6)
}

func (s *ImageService[T, P]) Upload(ctx context.Context, userID uint, owner OwnerRef, src io.Reader) (*T, error)
func (s *ImageService[T, P]) Get(ctx context.Context, userID uint, owner OwnerRef, id uint) (*T, error)
func (s *ImageService[T, P]) List(ctx context.Context, userID uint, owner OwnerRef) ([]T, error)
func (s *ImageService[T, P]) Delete(ctx context.Context, userID uint, owner OwnerRef, id uint) error
```

  The instantiations are `ImageService[ActivityImage, *ActivityImage]` and so on. `OwnerRef{ID uint; UID
  string}` comes from exactly one small per-kind function, `ownerResolver(tx, userID, idParam)`, which
  loads the parent `WHERE id = ? AND user_id = ?` (a contact resolves to its `vcard_uid`). That resolver and
  the three structs are the only per-kind code. The routes (a handler factory
  `imageRoutes[T, P](kind, resolver)`), the DTO, the sweeper and the cascade helper are written once.
- **Sweeper.** `imaging.Sweep` (decision 5) is one scheduled job over `IMAGES_DIR`, not three.
- **Cascade helper.** `services.imageCascadeStep(table, match)` builds the soft `CascadeStep` for an image
  table with its `Reason` filled in once, so the registrations in decision 4 cannot drift, and
  `services.collectImageFiles(tx, tables...)` plucks stored and thumbnail names for post-commit removal.

**`contacts.main_image_id`** is a nullable `INTEGER REFERENCES contact_images(id) ON DELETE SET NULL`,
added in slice (c) (SQLite allows `ADD COLUMN ... REFERENCES` with a NULL default). It points into
`contact_images` only. "Same contact, same user, live row" cannot be a plain FK, so it is enforced in the
service transaction that sets it and checked by a new invariant in
`backend/services/data_integrity_service.go` (ADR 0012): a main image that is soft-deleted, or belongs to
another contact or user, is a finding. A note's or activity's image is never a contact's main image
directly; *adopting* it copies it into `contact_images` (decision 10).

### 2. Format: a pure-Go re-encode, with no cgo and no WASM

`imaging.Process` accepts **JPEG, PNG and GIF** (first frame), using `image/jpeg`, `image/png`,
`image/gif` and `github.com/nfnt/resize`, all already in `go.mod`. In order:

1. **Sniff** with `http.DetectContentType` and reject SVG/HTML/XML signatures with the
   `hasMarkupSignature` rules now in `backend/controllers/attachment_controller.go` (moved into
   `imaging`). The client-declared type is never trusted.
2. **Header check** with `image.DecodeConfig`: reject when `width*height > MaxPixels` *before* any pixel
   allocation (the decompression-bomb guard, decision 6). The existing guard,
   `backend/photostore/photostore.go#CheckImageDimensions` at 100 megapixels, is too generous for a gallery
   with concurrent uploads, so images use their own limit.
3. **Bounded concurrency.** A process-wide semaphore of `IMAGE_PROCESS_CONCURRENCY` slots (default
   `min(2, GOMAXPROCS)`) wraps decode through encode. A request that cannot get a slot within 5 s gets
   `503` with `Retry-After`; nothing queues unboundedly. Worst-case decoded memory is
   `MaxPixels x 4 B x slots` (200 MB x 2 at the defaults), the number an operator sizes against.
4. **Decode, read EXIF, apply orientation 1-8** to the pixels. EXIF is read by a small in-repo reader
   (TIFF header, IFD0 `Orientation`, Exif IFD `DateTimeOriginal` and `OffsetTimeOriginal`) built on the
   JPEG segment walker in `exif_strip.go`, with strict bounds and a fuzz target. No new dependency.
5. **Resize** the full variant to a long edge of at most **2048 px, never enlarged**, and a **thumbnail**
   to a long edge of at most **320 px**.
6. **Encode** both as **JPEG** (quality 85 full, 80 thumbnail), or as **PNG** when the decoded source has
   any non-opaque pixel. The output carries **no metadata**: the re-encode drops GPS, serial numbers and
   the camera thumbnail by construction, which closes for gallery images the gap that
   `docs/security/asvs-l2.md` records as P10. The only retained fact is the capture day/instant.
7. **dHash** the oriented image (decision 9) and return it with dimensions, sizes and capture time.

The MIME type and extension are stored **per row**, and every reader serves the stored value, so a later
WebP encoder changes new rows only and needs no migration.

**Rejected input.** HEIC/HEIF is rejected with reason code `image_heic_unsupported` ("convert to JPEG
first"). An animated GIF becomes a still. WebP input is rejected (`image_unsupported_format`): the only
pure-Go decoder is `golang.org/x/image/webp`, a new module.

**Rejected encoders.**

- **WebP via a WASM encoder** (what the upstream fork chose) adds a second WASM runtime surface beyond the
  HEIC decoder, a new module through `docs/dependency-upgrade-policy.md` and
  `docs/security/dependency-exceptions.ignore`, a supply-chain and licence review, and a per-request
  instantiation cost, for a size win the 2048 px JPEG cap already mostly delivers.
- **cgo libwebp / libheif** breaks the static, cross-compiled, reproducible build with a pinned toolchain
  (`docs/security/reproducible-builds.md`, and the arm64 embedded backend of ADR 0028), and puts a C
  image codec - historically the largest source of memory-safety CVEs in a server - in the request path.

### 3. Account bundle: images travel in it, within the existing size cap

The bundle is the owner's full-fidelity backup (`backend/models/account_bundle.go#AccountBundle`, ADR 0028
decision 3), so images belong in it; leaving them out would be silent data loss. The layout is additive to
`version: 1`, the same precedent as `self_contact_uid` and `time_format`:

```json
"images": [
  { "uuid": "...", "owner_kind": "contact|note|activity",
    "owner": "<contact vcard_uid | note uuid | activity uuid>",
    "mime_type": "image/jpeg", "width": 2048, "height": 1365,
    "taken_at": null, "taken_date": "2024-07-03",
    "crop": {"x": 0.1, "y": 0.0, "w": 0.8, "h": 1.0}, "created_at": "...",
    "data": "<base64 of the full variant>" } ],
"contacts[].main_image": "<image uuid>",
"omitted": { "attachments": true, "photos_omitted": 0,
             "images_omitted": 0, "image_bytes_omitted": 0 }
```

- **Only the full variant is embedded.** The thumbnail is regenerated on import.
- **Budget.** `services.BuildAccountBundle` tracks the running encoded size against
  `services.MaxMycorrhizalBundleSize` (64 MiB, `backend/services/mycorrhizal_import_session.go`) less 4 MiB
  of JSON headroom, with a per-image ceiling of 4 MiB. Images are admitted **main images first, then newest
  first**. An image that does not fit is counted in `omitted.images_omitted` and `image_bytes_omitted`, and
  reported in a new `X-Mycorrhizal-Bundle-Images-Omitted` header beside the existing
  `X-Mycorrhizal-Bundle-Photos-Omitted`. Nothing is dropped silently and the export never fails because of
  images.
- **Import validates every image through `imaging.Process`.** The stored `mime_type`, `width` and `height`
  are never trusted: the bytes are re-decoded under the same pixel limit, concurrency bound and per-owner
  cap, re-encoded, and the thumbnail regenerated. An image that fails is skipped and counted in the import
  result (`images_rejected`); the import continues. An image whose `uuid` already exists for the user is
  skipped, so re-importing is idempotent. Owners resolve by the bundle's portable ids; an unresolved owner
  skips the image. `contacts[].main_image` is applied after all images land, and the avatar is regenerated
  only when the card carries no photo of its own.
- The flat CSV export carries no image data (it never carried attachments either); `docs/privacy.md` says
  the account bundle is how images are exported.

### 4. Delete, trash and purge

Image rows are **user-authored content and soft-delete** (ADR 0004, as `Attachment` does), decided once on
`ImageFields`. They have no natural-key unique index (dedupe is a distance test, decision 9), so no
partial index is needed.

| Where | Registration |
|---|---|
| `ContactCascadeRegistry` | `contact_images`: `CascadeSoft`, `Match` = by `contact_vcard_uid`, built by `imageCascadeStep`. `note_images`: `CascadeSoft`, `Match` = `note_id IN (SELECT id FROM notes WHERE contact_id = ? AND user_id = ?)`, ordered **before** the `notes` step. `activity_images` is **not** a contact-cascade step (fact 3). |
| Trash snapshot and restore | Nothing extra: both soft steps land in `ContactDeletionPayload.Soft` and restore by id. Files stay on disk while the contact is in the trash, so restore brings the pictures back. |
| `deleteUserCascade` | All three tables, `Unscoped()` hard delete (the ADR 0004 `DeleteUser` exception); file names plucked first and removed after commit. |
| Purge | `purgeContactInTx` adds the contact's `contact_images` and `note_images` file names to `PurgedFiles`; the soft-deleted model list in `backend/services/purge_service.go` gains all three tables, so soft-deleted note/activity images past retention are hard-deleted and their files collected. |
| Contact merge | The loser's `contact_images` are re-pointed to the keeper beside the attachments re-point in `backend/services/contact_merge_service.go`. The per-contact cap is not re-enforced on merge. The loser's `main_image_id` is dropped. |
| Note or activity delete | Soft-deleting the parent leaves its image rows and files in place, unreachable (every read joins to a live parent), until the parent is purged. |
| Deleting one image | Soft-deletes the row and, **after the transaction commits**, removes both files (the attachment precedent). If it was the contact's main image, the same transaction clears `main_image_id` and the derived avatar (decision 10). |

`backend/controllers/delete_cascade_coverage_test.go` gains one assertion: every table named `*_images` is
registered in exactly the places in this table.

### 5. Cleanup: crash-safe upload, post-commit removal, scheduled sweep

- **Upload order.** (1) Process in memory under the semaphore. (2) Write `*.tmp`, then rename. (3) Insert
  the row; if it fails, remove the files. A crash between (2) and (3) leaves files with no row.
- **Row deletion removes files only after the DB transaction commits**, never inside it; a removal failure
  is logged and left for the sweep, never rolled back (ADR 0037 decision 5).
- **Soft-deleted parents keep their images until purge**, so restore works.
- **Sweeper.** One job, registered with the lock and catch-up rules of ADR 0011
  (`backend/services/job_lock.go`), runs daily over `IMAGES_DIR`. A file is removed iff it is older than a
  1-hour grace period (an in-flight upload is never raced) **and** no row in any of the three tables, read
  `Unscoped()`, names it as `stored_name` or `thumb_name`. It also removes stale `*.tmp` files and
  soft-deletes image-less empty notes (decision 11). It never touches `PROFILE_PHOTO_DIR` or
  `ATTACHMENTS_DIR`; the same job shape can adopt them later. It records
  `{files_removed, bytes_removed}` in the job-run record, with no file names.
- `database.VerifyBackupSet` (`backend/database/backup_completeness.go`) and the restore drill learn
  `IMAGES_DIR`; `docs/deployment.md` and `docs/security/data-retention-lifecycle.md` section 5 document it.

### 6. Limits

| Limit | Default | Setting | Over the limit |
|---|---|---|---|
| Upload bytes (one file) | 20 MiB | `IMAGE_MAX_UPLOAD_MB` | `413`, `image_too_large` |
| Decoded pixels | 50,000,000 | `IMAGE_MAX_PIXELS` | `400`, `image_pixels_exceeded` |
| Live images per note | 10 | `IMAGE_MAX_PER_NOTE` | `409`, `image_limit_reached` |
| Live images per activity | 10 | `IMAGE_MAX_PER_ACTIVITY` | `409`, `image_limit_reached` |
| Live images per contact (own gallery) | 50 | `IMAGE_MAX_PER_CONTACT` | `409`, `image_limit_reached` |
| Concurrent decodes | `min(2, GOMAXPROCS)` | `IMAGE_PROCESS_CONCURRENCY` | wait up to 5 s, then `503`, `image_processing_busy` |

The aggregated Photos tab is not capped (it is a read). The request body is capped by
`middleware.BodySizeLimitMiddleware` (`backend/middleware/body_limit.go`) at the upload limit plus
multipart overhead. The reason codes are new `backend/errors/reasons.go` entries per ADR 0038
(`image_too_large`, `image_pixels_exceeded`, `image_unsupported_format`, `image_heic_unsupported`,
`image_corrupt`, `image_limit_reached`, `image_processing_busy`), with the limit in `reason_params`.

**Per-user quota.** Image bytes (`size_bytes + thumb_size_bytes`, all three tables) count toward the
existing opt-in `PER_USER_ATTACHMENT_QUOTA_MB`: `UserQuota.CheckAttachmentUpload`
(`backend/services/user_quota.go`) is generalized to sum attachments and images, and the setting keeps its
name (a rename is a breaking config change). A `diskspace.Require` preflight
(`backend/internal/diskspace/diskspace.go#Require`) on `IMAGES_DIR` precedes the write and returns the
existing 507.

**Storage metrics.** `systemStatusStorage` (`backend/controllers/system_status_controller.go`) adds
`IMAGES_DIR` to its `Directories` and an additive `images` object
`{count, bytes, by_kind: {activity, note, contact}}`, summed from the tables over live and not-yet-purged
soft-deleted rows (both occupy disk). `storage_samples` gains `image_dir_bytes`
(`backend/models/storage_sample.go`, filled by `storageDirBytes` in
`backend/services/storage_sample_service.go`) so growth projections include images.

### 7. Sensitivity: inherited from the parent, filtered in the query

An image has **no sensitivity column**. Its effective sensitivity is its parent's, through one function,
`models.ImageSensitivity(kind, owner)`. Today it returns `normal` for every parent (fact 2). When a parent
gains a sensitivity (or an activity takes the strictest of its attendees), that one function changes and
no image row does.

- **Nothing that leaves the instance carries image bytes** except (i) the **derived avatar** and (ii) the
  **account bundle**, a deliberate full-fidelity owner-backup exception like the CSV and the contact map
  (backend/AGENTS.md). CardDAV/CalDAV sync, contact shares, the MCP server, vCard and JSContact export, CSV
  and webhooks do not emit gallery, note or activity images. Adding one is a policy change that filters in
  the query and takes that surface's include flag.
- **The CardDAV/vCard/JSContact/export avatar is derived only from the main image of a `normal`
  contact.** The derivation writes `contacts.photo` (decision 10); outward builders reach it only through
  `backend/models/contact_record.go#RecordForContactFiltered`, which is where a sensitivity test belongs,
  and `backend/models/contact_record_outward_census_test.go` already forces every builder to be classified.
- **URLs are owner-scoped and authenticated** (decision 8). There is no public, signed or guessable file
  path, and no stored name is ever returned.

### 8. API shape: owner-nested routes

All routes sit under the owner, in the existing `protected` group (the same session/bearer auth as
attachments and the profile picture, `backend/routes/routes.go`):

| Method and path | Purpose |
|---|---|
| `POST /{contacts\|notes\|activities}/:id/images` | multipart upload, field `file`; `201` with the image DTO. A dHash duplicate on a contact returns `200` with the existing image and `deduplicated: true`. |
| `GET  /{...}/:id/images` | `{images: [...], total}`; the slice has no `omitempty` (frontend trap #8) |
| `GET  /{...}/:id/images/:image_id/file` | full variant bytes |
| `GET  /{...}/:id/images/:image_id/thumbnail` | thumbnail bytes |
| `PATCH /{...}/:id/images/:image_id` | allowlisted fields only: `crop` (null clears) |
| `DELETE /{...}/:id/images/:image_id` | soft delete, then post-commit file removal |
| `PUT` / `DELETE /contacts/:id/main-image` | set (`{image_id}`) or clear the main image (slice c) |
| `POST /contacts/:id/images/adopt` | copy a note's or activity's image into `contact_images`: `{source_kind, source_id, image_id}` (slice c) |
| `GET  /contacts/:id/photos` | the Photos tab: own, notes' and activities' images, newest first by `COALESCE(taken_at, created_at)`, keyset-paginated, each item labelled with `source: {kind, id, label}` (slice c) |

**Why owner-nested and not a flat `/images/:id`.** Three tables have overlapping integer ids, so a flat
route needs a kind discriminator in the id or the path - a polymorphic lookup, the thing decision 1 avoids.
Nested, the ownership check is one indexed query (`parent WHERE id AND user_id`, then
`image WHERE id AND <owner key> AND user_id`); an image of a foreign owner or another parent is a `404`,
never `403`, as for every other resource; the OpenAPI groups are symmetric; and one handler factory takes
the owner from the path. The cost - a client needs the owner to form a URL - is paid by the DTO: every
image carries ready-made relative `file_url` and `thumbnail_url`, so no client assembles one. A note's or
activity's JSON gains an additive `images: [...]` (always present, possibly empty), batch-loaded with one
`IN (...)` query per list response, with no N+1.

**Ownership and mutation.** Ownership is enforced in the service (`Get` and `Delete` take `userID` and the
`OwnerRef`), not only the handler, and every query carries `user_id`. `PATCH` is an explicit allowlist (no
mass assignment). Create and delete record audit events. Setting or clearing a main image updates the
contact in one transaction and bumps its revision (ADR 0006, ADR 0018).

**Download headers** (both variants): `Content-Type` is the **stored** `mime_type` (`image/jpeg` or
`image/png` only); `Content-Disposition: inline; filename="image-<uuid>.<ext>"`, or `attachment` with
`?download=1`; `X-Content-Type-Options: nosniff`; `Content-Security-Policy: default-src 'none'; sandbox`;
`Cross-Origin-Resource-Policy: same-origin`; `Cache-Control: private, no-cache` with a strong `ETag` from
the row `uuid` and variant, and `If-None-Match` answered `304` only after the ownership check, so a cache
never bypasses authorization. The original filename is never echoed.

### 9. Dedupe: dHash, per contact, Hamming distance of 4 or less

The pipeline stores a 64-bit **difference hash** (9x8 grayscale, adjacent-column comparison) of the
EXIF-oriented image on every row. **Dedupe applies to `contact_images` only**: before inserting, the
service compares the new hash with the contact's live images (at most 50 rows, popcount in Go) and, at a
distance of **4 or fewer**, returns the existing image instead of inserting. Re-encodes and resizes of one
picture land at 0-2; unrelated pictures of the same person, even bursts, sit well above 8, so 4 keeps a
margin without merging distinct photos. The threshold is a named constant (`imaging.DedupeMaxHamming`),
not a setting.

It applies to every automatic ingress - the `PHOTO` of an inbound CardDAV/vCard card, a source import,
bundle import, profile-photo adoption and `adopt` - so a returning copy of the current picture is a no-op.
A *different* inbound avatar is not a duplicate: it becomes a new contact image and the new main image
(today it overwrites the single photo; now the old one stays in the gallery). Manual uploads use the same
rule, which also makes a retried upload idempotent. Note and activity images are not deduped (the same
screenshot on two notes is legitimate); their hash is stored so the rule can widen later.

### 10. Avatar compatibility and the contact gallery

`contacts.photo` and `PhotoThumbnail` **stay**, and remain the only thing CardDAV, vCard, JSContact, the
CSV and the bundle's card read, so none of them changes. When a main image is chosen or cropped, the
service regenerates both from the stored full variant plus crop at the existing profile size (400 px
square-cropped, 48 px thumbnail, as `processAndSavePhoto` does), updates the contact in one transaction and
removes the old avatar files after commit (the current `AddPhotoToContact` sequence).
`POST /contacts/:id/photo` becomes sugar over the gallery in slice (c): its upload goes through
`ImageService[ContactImage]` (so it dedupes and appears in the gallery) and is set main. Its decoder set
still includes HEIC through the existing dependency, so no avatar upload regresses.

- **Existing profile photos are untouched by every migration.** A contact with `photo` set and a NULL
  `main_image_id` shows the legacy photo as a read-only "profile photo" tile in the Photos tab.
- **Adoption is optional.** A background job, off by default (`IMAGES_ADOPT_PROFILE_PHOTOS`), inserts a
  `contact_images` row for each such contact from the stored photo and sets it main without regenerating
  the avatar. It is idempotent, batch-limited, dHash-deduped and uses the ADR 0011 catch-up lock. Adoption
  also happens naturally the first time the user changes the picture.
- **Deleting or clearing the main image** clears `main_image_id` *and* the derived avatar (`photo`,
  `PhotoThumbnail`), because a deleted picture must not linger in CardDAV. A legacy photo with no main
  image is never touched.
- **Crop** is stored as fractions of the full variant and applied only when deriving the avatar; the
  stored image is never cropped destructively.
- **Adopt** copies the *stored full variant* into `contact_images` with no re-decode, so a main image
  always lives in a contact-owned row with the contact's own lifecycle.

### 11. Notes: image-only notes

`Note.Content` is `validate:"required,min=1"` today (`backend/models/note.go#Note`). The invariant becomes
**non-empty content, or at least one live image**. Uploads need a note id, so an image-only note is
created in two steps:

- `POST /notes` accepts `content: ""` only with `awaiting_images: true` (a new optional write-only field);
  the note is then valid for a **1-hour grace window**.
- A note with empty content and no live image past the window is soft-deleted by the sweeper (decision 5)
  and counted, so a crashed client leaves nothing visible.
- `PUT /notes/:id` with empty content is refused (`422`) unless the note has a live image, and deleting the
  last image of an empty-content note is refused (`409`, `note_requires_content_or_image`).
- The upload response carries `taken_date`; a client may offer to set the note's date to the **first
  image's `taken_date`**, a calendar day applied with no zone conversion (ADR 0015 category 2). The server
  never rewrites `Note.Date` on its own.

Rejected: a multipart `POST /notes` carrying text and files, which forks the note create path that today
owns idempotency, `If-Match`, quota, webhooks and the revision stamp. Also rejected: ownerless "pending"
image rows claimed later, which would need a nullable owner and bring back the polymorphic table.

## Implementation order (v1.6.0, all three ship)

1. **(a) Shared components and activity images (#1994), backend and web.** `activity_images` migration;
   `ImageFields`, `imaging` (pipeline, store, EXIF, dHash, limits), `ImageService`, the sweeper job,
   config, reason codes, routes, `deleteUserCascade` and purge registration, quota and metrics, bundle
   export and import of images, activity form and timeline UI.
2. **(b) Note images**, including image-only notes: `note_images`, the content-or-image invariant,
   `awaiting_images`, the contact-cascade step, the note editor UI.
3. **(c) Contact gallery:** `contact_images`, `contacts.main_image_id`, main/crop/adopt routes, avatar
   derivation and the legacy-photo bridge, dedupe on inbound CardDAV and import photos, `GET
   /contacts/:id/photos`, the Photos tab (a `SectionGroup` id and a `ContactJumpNav` entry), the optional
   adoption job, merge re-point, and the CardDAV/export-unchanged tests.
4. **Android follows each slice**, after the backend and OpenAPI for that slice merge.

Each slice has its own migration with a symmetric `.down.sql` (the down drops the table; image files stay
for the operator and the sweeper, and the release note says so), a schema-parity registration, and a
`migrate_datapreservation` test proving existing attachments, photos and notes are untouched.

## Consequences

- Three new tables over time and one new directory (`IMAGES_DIR`), included in the backup docs and
  `VerifyBackupSet`.
- One new in-repo parser (EXIF) on an untrusted-input path: bounded, fuzzed, and covered by the tests
  below. No new module, no cgo, and no WASM beyond what the avatar path already has.
- The bundle can grow by up to the cap; its omission counters make a truncated export visible.
- Stored pictures are the 2048 px re-encode; the camera original is not kept.
- Slice (a) updates `docs/security/asvs-l2.md` (upload rows and P10), `docs/security/threat-model.md`,
  `docs/security/data-retention-lifecycle.md` section 5 and `docs/security/pii-inventory.md` (a new
  persistent copy of user data). This ADR does not, because nothing is implemented yet.
- Security tests required in slice (a), per `.agents/skills/security-sensitive-change/SKILL.md`: a foreign
  user and a foreign parent each get `404` on every route; path traversal in every stored-name position;
  an SVG/HTML upload; a decompression bomb and a truncated JPEG, plus an EXIF fuzz corpus; GPS EXIF absent
  from stored bytes; oversize bytes and pixels; the per-owner cap; a user-delete test asserting rows and
  files are gone; and a trash-restore test that brings images back.

## Open questions for the maintainer

1. **HEIC asymmetry.** The avatar upload accepts HEIC today (existing `gen2brain/heic`, WASM) while the
   gallery rejects it, so an iPhone HEIC works as an avatar but not as a gallery image. Recommendation:
   keep it rejected for v1.6.0 and revisit once the pipeline exists; enabling it is a decoder-set change
   with no schema work.
2. **Parent sensitivity does not exist.** Decision 7 is correct but, today, vacuous: contacts, notes and
   activities have no sensitivity. If the intent is that a user can mark a note or contact private or
   secret, that is a separate, larger decision (it changes sync, shares, MCP and the census) and needs its
   own issue; this ADR needs no change when it lands.
3. **Originals.** Only the 2048 px re-encode is kept. A user who needs the original camera file has the
   existing attachment feature; confirm that is acceptable rather than keeping a third variant.
