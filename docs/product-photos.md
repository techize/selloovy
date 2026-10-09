# Product cover photos

After explicit migration to schema 9 and an admin rebuild, open Products → Photo.
The editor shows the current product name and number; verify these before
choosing an image. Changing a product name or description keeps its existing photo.
Choose a JPEG or PNG and enter descriptive alt text (1–160 plain-text characters).
Save the draft photo, then choose Review & publish in the same editor and
publish/republish the reviewed content. Back to editor reloads the latest saved
revision. Unsaved photo changes must be saved before opening the review.
One cover photo per product appears on its public page and catalogue card.

Alt text can be edited without choosing another file or recompressing its image bytes. Removing or replacing a
draft photo leaves the published image and description unchanged until republish.
Unpublishing removes public image access immediately. No real catalogue images
are seeded or automatically copied into the public repository.

## Upload and access boundaries

Both original and canonical output are limited to 5 MiB. Dimensions are checked
before pixel allocation: at most 4,096 per side and eight million pixels.
Only decoded JPEG and PNG are accepted; filenames, extension and uploaded MIME
claims do not select the decoder. Empty/malformed/SVG/unsupported images fail.
Processing allows at most two concurrent conversions per process.

Images are decoded and re-encoded, discarding original filename and embedded
metadata. JPEG EXIF orientation 1–8 is applied first. JPEG output uses quality 85;
PNG transparency is retained. Original bytes and metadata are not persisted.
Colour-profile conversion, PNG orientation metadata, cropping, responsive
derivatives, galleries and per-variant images remain later work.

Multipart is permitted only for PUT /api/admin/products/{id}/photo. Exact host,
Origin, custom request header, session validation and five-second request deadline
remain required. Multipart elsewhere remains rejected. Upload bodies, field sizes
and duplicate/unknown fields are bounded or rejected. Private image requests check
the current owner/product; public requests require the shop key, published product
and that publication's exact photo ID. Responses are uncached and nosniff.

## Storage and transactions

The POC stores canonical image bytes in PostgreSQL, capped per photo. Database
backup and restore therefore include photos without a separate filesystem volume.
Storage size must be measured before a larger merchant deployment; object storage
and derivative generation can be introduced through the catalogue boundary later.
No storage-provider credentials or paid services are required here.

Each save creates immutable content, checks the shared product revision and
updates the draft selection atomically. The published projection retains its own
photo reference and reviewed alt text. A stale upload conflicts; uncertain saves
require reloading. Unreferenced older images are removed in the same transaction
when selections/publications change; images still referenced by a draft or published
projection remain. Authenticated actor/stock history is not used for media.

Schema 9 adds product_photos and nullable draft/publication references. Existing
drafts and published products continue with no photo until the merchant adds one.
Database migrations remain explicit and forward-only.

Verification includes byte/dimension/format limits, metadata removal, transparency,
all JPEG orientation transforms, ownership, exact multipart scope/origin/session
checks, draft/public image separation, alt editing, stale/racing saves, rollback,
reference cleanup, republish/removal, unpublish/logout and outage. Browser QA uses
synthetic image/product data, checks upload and reload, alt validation/editing,
review/publish, catalogue and product display, draft removal/republish and 390px
layout. The real merchant installation is not used for these operations.
