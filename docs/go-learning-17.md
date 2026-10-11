# Go lesson 17: a draft is not a publication

Page and Section structs describe allowed content. JSON tags name API fields;
validation enforces the four kinds, text limits, palettes and internal links before
storage. Go html/template escapes merchant text rather than treating it as code.

ChangePage starts a transaction, locks the owner/session and shared shop revision,
and checks the caller's revision. Save changes only the draft array. Publish copies
a saved draft into the publication array after checking product ownership and
publication. Commit makes the full change visible together; a failed write rolls
back the revision as well as content. A transaction keeps two concurrent publishing
requests from both succeeding with the same revision.

The storefront depends on a small PagesBackend interface. Supplying this capability
adds published pages without requiring existing catalogue test doubles to implement
it. Rendered sections embed the structured Section and add resolved public products.
A per-request map reuses product lookups across sections and records missing products.

Optional exercise: trace saving a changed hero heading without publishing, then
republishing it. Explain why the old heading remains live between those actions and
why publishing cannot make a foreign-shop product visible.
