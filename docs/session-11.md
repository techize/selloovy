# Increment 11: reviewed publishing and public browsing

Delivered owner-controlled publish/republish/unpublish and Go-rendered public
shop/product pages. Review includes shop identity and current product/variants.
Public projections hold reviewed prices and content; availability reads current
stock/supply/fallback. Drafts and new options remain private until publication.
Public routes exclude contact emails, authentication data and stock counts.

Schema 8 adds shop/product publication tables. Session-owned transactions check
product and shop revisions, advance the shared product revision and commit
identity/content together. Concurrent actions conflict. Public reads are scoped
by opaque shop key plus publication, bounded and uncached. Plain text is escaped.
The storefront supports variant selection without JavaScript and displays working
day duration defaults and certificate policy. Ordering stays disabled.

Verification: PostgreSQL-backed Go race tests, vet/module/build checks, sqlc
regeneration, Vue type/build checks and isolated synthetic browser verification.
Tests include cross-shop access, stale product/shop identity, publication rollback,
unchanged draft snapshots, live availability, new-option privacy, variant prices,
escaping, public exclusions, unpublish, pagination, logout/outage. Browser confirms
publication persistence, stock and production lead times, price selection,
public listing, unpublication confirmation and mobile width. HTTP checks verify
unpublished routes return 404. Public scans and final-head CI required before merge.

Next: reviewed photo upload/display, then basket with certificate owner names.
Custom-domain/homepage shop binding, section builder, shipping/calendar dates,
Square 3DS proof, stock holds/orders and notifications remain open. Go lesson 11
explains public DTOs and immutable projections.
