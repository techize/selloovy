# Go lesson 11: publish a projection

A Go struct can define exactly what crosses a boundary. PublicProduct and
PublicVariant contain customer-visible content, without embedding the admin
product or its stock counts. Building them explicitly makes accidental disclosure
harder than serializing a database row.

SavePublication validates session ownership and reviewed revisions in a
transaction, then JSON-encodes that projection with json.Marshal. The database
stores it independently of the editable draft. Publishing advances the product
revision so concurrent publish/unpublish actions cannot silently overwrite one
another. Rollback covers the revision, identity and snapshot together.

PublicRead combines the snapshot with current availability in a repeatable-read
transaction. Labels and prices are reviewed content; stock and supply policy are
live facts. A new variant cannot appear just because it exists in the private draft.
The public type never includes its exact stock count.

The storefront handler accepts an interface with only two public read methods.
html/template escapes merchant text, and a buffer finishes rendering before the
HTTP response is written. Failed reads/rendering return generic errors.

Optional exercise: explain why calling ReadProduct directly from an anonymous
HTTP handler would cross the wrong boundary, even if its output happened to look
correct today. Then trace a stale publish through ErrConflict to HTTP 409.
