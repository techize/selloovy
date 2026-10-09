# Increment 14: photo editor product context

The photo editor now identifies its product in the heading, description, form
legend and save/remove confirmations. A visible product number helps distinguish
similarly named items. Guidance explains that renaming a product keeps its photo;
a replacement must be selected explicitly.

The editor loads the current product name through the existing owner-scoped
product endpoint and verifies that the product and photo revisions match before
enabling saves. A changed or failed read requires reloading. No schema or backend
changes, automatic photo reassignment or merchant content changes are introduced.

Verification: Vue typecheck and production build; isolated PostgreSQL-backed
browser checks with two synthetic products and distinct photos, named save
confirmation, switching between editors and current name after renaming a product.
Fixture database, service, files and tab were cleaned up. Publication guard,
manual staged review and redacted secret scans run before publication.

Go learning connection: the shared product revision acts as a consistency check
across the two owner-scoped reads. The existing Go save transaction checks that
revision again before writing; the UI check improves review context.

Next: basket with certificate owner-name collection and deterministic prices.
