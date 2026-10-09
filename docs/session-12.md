# Increment 12: product cover photos

Delivered one cover photo per product with descriptive alt text. Owner upload,
alt-only editing and draft removal join the shared product revision. Publish
review includes the saved photo; public product pages and catalogue cards render
the reviewed image and alt text. Draft replacement/removal does not change the
public image until republish. Unpublishing revokes public image access.

Schema 9 adds bounded immutable media in PostgreSQL, with draft/publication
references and transactional cleanup of unused content. No external storage
configuration, runtime dependency or image fixture is shipped. JPEG/PNG decode,
dimension/byte limits, metadata stripping, JPEG orientation and PNG transparency
are covered. Multipart is narrowly enabled for photo PUT without weakening
host/origin/custom-header/session checks.

Verification: full Go race/PostgreSQL suite, vet/build/modules, sqlc drift, Vue
types/build, publication guard and redacted secret scans before publication.
Tests cover image validation/transforms, ownership, multipart boundaries, draft
and published separation, alt editing, racing/stale saves, induced rollback,
cleanup, republish/removal, unpublish/logout/outage. Isolated browser checks cover
actual upload/preview/save, alt editing/validation, reload, publish review, loaded
product/catalogue images, draft removal then republish, and mobile width.

Next: basket with certificate owner-name collection and deterministic prices.
Galleries/per-variant photos, custom domains/homepage binding, shipping/calendar,
section builder, Square strict 3DS proof, inventory holds/orders and notifications
remain open. Photos are included in database backup/restore; measured storage and
object-storage/derivative work follow before broader scale.
