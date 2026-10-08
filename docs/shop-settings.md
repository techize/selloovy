# Authenticated shop identity settings

Owners can edit their shop name (required, 1–120 Unicode characters), tagline
(optional, up to 160), plain-text description (optional, up to 2,000) and customer
contact email (optional). Unicode is preserved; surrounding whitespace is trimmed.
Descriptions allow line breaks; control characters and display-name email forms
are rejected. Email is normalized consistently with owner addresses.

Settings remain private in admin. Saving does not publish the contact address or
change the public storefront. UK/GBP/Europe-London defaults are displayed read-only;
VAT, pricing, shipping, media and storefront publishing are subsequent increments.
No customer message is sent when a contact email is saved.

## API and ownership

`GET /api/admin/shop` reads the authenticated owner's shop. `PUT /api/admin/shop`
accepts name, tagline, description, contactEmail and the revision loaded by GET.
There is no client-supplied owner/shop selector. Unknown JSON fields or multiple
JSON values are rejected, and the request body is limited to 16 KiB.

The shared authentication wrapper applies configured Host/Origin, custom-header,
JSON, cookie, no-store and five-second request controls. Commerce SQL revalidates
the purpose-bound session digest, credential version and idle/absolute expiry.
The store locks owner/session and shop state, compares the expected revision, then
updates the identity and revision in one transaction. Concurrent logout/MFA
activation and saves serialize through those locks. Settings queries live in
shop.sql and generated internal/shopdb; credential queries remain in authdb.

Invalid fields return 422 with safe field messages. Stale revisions return 409;
the form retains its draft and asks the owner to reload saved details. Session
failure returns 401; dependency failure returns a generic 503. An interrupted
commit may have succeeded: the UI reloads authoritative state before another save,
rather than retrying blindly. No merchant values or tokens are logged.

## Verification and next work

Disposable PostgreSQL/HTTP tests cover two-shop isolation, client selector refusal,
origin checks, body limits, validation, persistence, stale edits, two-client races,
logout denial and storage outage. Browser QA checks save/reload, inline validation,
stale-edit feedback in a second tab and mobile overflow with synthetic records.
Schema 5 preserves existing names and adds identity fields plus revision defaults.

Next: product creation/listing, followed by maker variants and the section builder.
The complete setup-to-test-order POC and Square enforcement proof remain open.
