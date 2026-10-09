# Shipping services and working-day previews

Schema 11 adds shop shipping configuration, a basket's reviewed service choice
and per-product made-to-order preparation. Settings → Shipping supports up to ten
services, add/edit/remove/disable, final GBP charges, 1–30 working-day transit ranges
and an optional inclusive free-delivery threshold. All services currently cover all
UK addresses. No carrier API, label purchase, postcode zones, VAT or address capture
is added. Charges are merchant settings, not fetched carrier quotes.

Use UK starter services is an explicit draft action: Standard £4.99, free from
£30.00, transit 3–5 working days; Tracked 48 £5.99 / 2–3; Tracked 24 £8.99 / 1–2.
Review/edit and save before applying. Migration leaves all shops with no services;
it does not seed a merchant business or change existing product data.

## Charges and eligibility

Thresholds use the qualifying merchandise subtotal, excluding shipping. There is
no discount engine yet; a future discount must adjust that basis before quoting.
£29.99 pays £4.99 standard; £30.00 and above is free. Upgrades retain their charges
when no threshold is configured on them. Shipping fees and totals use integer pence.
Disabled/deleted/unknown services cannot be selected; an existing selection is
flagged unavailable and needs replacing. Changed charges or transit settings are
shown with a review notice. Selection checks basket and current shop revisions;
quantity or configuration changes require reloading a stale form.

Stock is checked across all lines sharing a variant, including different certificate
owners. If combined demand exceeds physical stock, product-controlled fallback
uses made-to-order preparation; otherwise the basket is blocked but quantities can
be edited. These checks do not reserve stock. A final recheck/hold and payment quote
remain required. Blocked or empty baskets cannot choose shipping.

## Preparation and calendars

Products → Variants & stock edits made-to-order preparation (1–90 working days;
default 5–7). Explicit product overrides support 3–5-day items. Missing bounds in
older API clients preserve the existing values; inverted/partial bounds are rejected.
Stocked dispatch uses the existing two-working-day estimate. Combined dispatch
uses the largest minimum and maximum among all lines, including fallback demand.
Faster transit does not shorten preparation. Preparation changes update current
catalogue/basket estimates without republishing product text.

Preview dates assume successful payment today in Europe/London. Day one is the next
Monday–Friday date excluding England and Wales bank holidays, regardless of hour.
Transit starts on the next eligible working day after estimated dispatch. London
zone data is included for minimal containers. Dates are estimates, not guarantees;
no payment/paid order or accepted purchase promise is created by this preview.

The bundled calendar covers 2026–2028, checked against the [GOV.UK calendar](https://www.gov.uk/bank-holidays)
and [JSON feed](https://www.gov.uk/bank-holidays.json) on 9 October 2026. It includes
substitute weekdays and skips both weekends and excluded dates. Contains public
sector information licensed under the [Open Government Licence v3.0](https://www.nationalarchives.gov.uk/doc/open-government-licence/version/3/).

Operators can supply a reviewed GOV.UK-format local JSON file using
SELLOOVY_BANK_HOLIDAY_FILE and restart, without an application release. Only England
and Wales events are used. Files are bounded, dates parsed and the year span checked
for gaps; this cannot prove that an operator file contains every correct holiday.
Keep the data current after announcements and before expiry. No network request
occurs during a basket/checkout request. Invalid configured files stop startup;
unsupported date ranges show no calendar promise instead of guessing. An automated
refresh process and regional/carrier calendars remain later work.

## Access and later order promises

Owner shipping changes use existing session/MFA, exact-origin JSON and shared shop
revision/lock boundaries. Basket service choice uses the existing cookie/CSRF/native
form controls. Configuration and basket writes are atomic; failure rolls back.
No credentials, addresses or customer details are part of public shipping settings.

Shipping estimates are live basket previews. Future orders must revalidate the
quote at payment, anchor preparation to trusted payment time and retain immutable
service/charge/preparation/transit/calendar/date snapshots. Later changes must not
rewrite those accepted promises. Checkout/address capture, stock holds, order
snapshots, Square strict 3DS proof, notifications and abandonment admin remain open.
