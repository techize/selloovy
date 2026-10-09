# Increment 15: durable guest basket

Delivered server-rendered add/update/remove forms, private shop/browser baskets,
certificate owner-name collection and deterministic published-price subtotals.
Schema 10 stores bounded basket documents with hashed bearer tokens, optimistic
revisions and fixed seven-day expiry. No JavaScript, external provider or paid
service is added. Checkout remains visibly disabled.

Full Go race/PostgreSQL suite, vet/build/modules and Vue type/build passed.
Focused tests cover required/unsupported/control/long names, quantity limits,
matching/different owners, browser/shop/variant isolation, restart-independent
storage, draft/public price separation, reprice notices, duplicate/racing writes,
induced rollback, cookie/origin/CSRF/field boundaries, escaped names, HTTPS cookie
protection, unpublish/removal and expiry. Generated queries are regenerated and
reviewed; publication guard and redacted scans run before publishing.

Isolated browser QA verifies required-name validation, successful native add with
redirect, quantities and exact totals, certificate-free products, a mixed basket,
reload persistence and removal. Referrer-Policy changed to same-origin after
browser testing exposed Origin:null with no-referrer for form navigation; strict
origin enforcement stays. A viewport override did not change the browser's actual
1280px width, so no mobile-width validation is claimed. Synthetic fixture DB,
process/files/tabs were removed and the override reset.

Next: shipping services and working-day fulfilment; then section builder, inventory
holds/orders, Square strict 3DS proof and confirmations/abandoned-basket admin.
Scheduled expiry cleanup and rate controls are required before live release.
