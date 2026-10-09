# Product publishing and public catalogue

After applying schema 8 and rebuilding admin, choose Products → Publish & preview.
Review the current shop identity, product content and saved options, then choose
Publish reviewed product. At least one saved variant is required. All prices
remain final GBP product prices; shipping is not yet calculated.

The generated public product link also leads back to its shop catalogue.
These are deliberately public pages, not secret previews. An opaque shop key
identifies the route; it is not authentication. Custom domains and routing the
installation homepage to a shop remain future work. The foundation homepage
still serves the root route.

## Explicit public projection

A publish transaction snapshots name, description, certificate requirement,
variant IDs/labels/size/colour, effective prices and the reviewed cover photo/alt text. Ordinary draft changes do not
change those public fields. Republish updates the snapshot; newly added variants
remain private until then. The admin flags newer saved product revisions.

Publishing also refreshes the shop's shared public name, tagline and description
for every published product in that shop. The reviewed shop revision must match.
Contact email, owner credentials, creation keys, stock counts and adjustment
history are excluded. Publishing and unpublishing advance the shared product
revision, preventing stale tabs from overwriting a newer publication action.

Availability uses current stock, supply mode and product fallback for reviewed
variants. Stocked positive counts show dispatch within two working days; production
shows 5–7 working days and shipping a further 3–4. Those are current duration
defaults, not calculated calendar dates or bank-holiday-aware promises.
Snapshot content and live availability are read in one repeatable-read transaction.
An unavailable option is still browsable.

Unpublish deletes the public product projection. Subsequent requests return 404,
and the catalogue excludes it. With no published products the shop route returns
404 too. Responses use no-store, escaped plain-text templates and a restrictive
CSP; already viewed content cannot be recalled. Public queries always require the
shop key and published product scope. Catalogue pages contain at most 50 products,
with a cursor for the next page. Invalid/foreign variants return 404. Outages
return a generic 503 without connection details.

## Remaining work

Reviewed [cover photo uploads](product-photos.md), [guest baskets and certificate
owner-name collection](guest-basket.md) are available. Editable shipping/calendar previews are available. Section builder, stock holds,
payment and order confirmation remain
required for the POC. Checkout is visibly disabled; no card data is collected.
Square's strict default 3DS proof remains open. No live commerce readiness claim.

Verification covers authenticated ownership/origin/body boundaries, missing
variants, stale/racing publication, atomic rollback, persistence, reviewed content,
live availability, price selection, escaping, private-field exclusions,
unpublishing, pagination, logout and outage. Browser QA uses a separate synthetic
database and loopback origin, including desktop and 390px layout.
