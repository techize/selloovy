# Go lesson 08: save the right shop without losing edits

`internal/shop` owns identity rules and persistence. `internal/shophttp` translates
requests and errors. Vue edits a small JSON model. This separation lets the rules
stay in Go while the browser controls the form experience.

The session is the shop selector. The client sends an opaque cookie; a purpose-bound
digest joins the saved session to its owner and shop. The API never accepts a shop
ID from the form. This is a concrete alternative to looking up an arbitrary Rails
model ID from request parameters and hoping a later permission check catches it.

Go's `Input` and `Settings` structs have deliberate JSON fields. Input omits country,
currency, timezone and IDs; unknown fields fail decoding. A `ValidationError` carries
field messages, and `errors.As` lets HTTP recognize it without exposing driver errors.
Go counts Unicode code points, so eight bytes or UTF-16 units are not treated as eight
characters. The same principle applies to the shop name and description limits.

Revision numbers prevent lost updates. Both tabs may read revision 1. The first save
locks its owner/session and shop, compares 1, updates the fields and returns 2. The
second save then sees 2 and returns a conflict instead of replacing the first edit.
The browser keeps its draft until the owner deliberately reloads saved details.

Authorization is checked inside commerce SQL too. The authentication wrapper checks
access before the handler, but logout or MFA activation could happen afterward. Store
transactions revalidate and lock the session/owner, so the save has an authoritative
permission boundary rather than relying only on an earlier middleware result.

`defer` releases the transaction on every early error. Commit establishes the saved
revision; cancellation or a lost response can leave its outcome uncertain. The UI asks
for a reload instead of assuming every failure means the write was rolled back.

Optional exercise: trace `TestOwnedShopSettingsPersistenceAndIsolation`. Explain why
a second merchant cannot choose the first shop, then predict the two possible winners
when two clients save the same revision. Both outcomes must preserve one complete edit.
