# Increment 16: shipping services and working-day estimates

Delivered editable owner shipping services and a one-click reviewed starter draft,
basket delivery choice/charges/combined totals, threshold rules, service-change
notices and disabled-service replacement. Per-product preparation overrides join
variant editing. Combined stock demand checks include personalised lines and use
fallback preparation or block the basket. No stock is reserved and checkout stays
disabled. Schema 11 preserves existing catalogue/basket/authentication data and
starts shops with no shipping services.

Calendar previews use Europe/London, the next eligible working day, weekends and
England/Wales bank holidays from GOV.UK (2026–2028). Operators can refresh from a
reviewed file without code changes. Unsupported ranges suppress dates and invalid
files stop startup. Paid-order snapshots/confirmed-payment anchoring remain open.

Full Go race/PostgreSQL, vet/build/modules and Vue types/build passed. Focused tests
cover inclusive thresholds and paid upgrades, calendar holidays/substitute days,
weekends/London date boundaries, expiry and file maintenance, config validation,
owner/foreign-shop/origin/unknown-field boundaries, stale settings, failed-write
rollback, selection/repricing/disabled/unknown services, partial stock, combined
personalised demand, blocked-basket selection and preparation override preservation.
Owned baskets remain readable/removable after the final product is unpublished;
the ordinary catalogue stays hidden. This regression is covered by a focused test.
Generated queries are reviewed and publication scans run before publication.

Isolated browser QA covers service editing and starter draft/save, charged/free
standard delivery at exactly £30, paid upgrade, mixed-order dates, disabled-service
replacement and saved preparation changes updating the public basket. Saved
availability remains unchanged while preparation fields are still an unsaved draft.
Fixture database/process/files/tabs removed; synthetic screenshot kept privately.
No mobile-width validation is claimed. No real merchant settings/data are seeded.

Next: required section drag-and-drop builder, stock holds/order snapshots, Square
strict 3DS evidence, confirmations and abandoned-basket admin. Scheduled expiry
cleanup, rate controls and full live readiness still remain.
