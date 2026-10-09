# Increment 13: editor feedback and publication flow

Product and cover-photo editors now open publication review inline using Review
& publish. Merchants can publish or republish without returning to the product
list. Review uses the existing consistent saved snapshot and revision checks;
unsaved edits must be saved first. Editing controls are disabled during review,
and Back to editor reloads the latest saved revision after publication.

Save success/error feedback sits beside the save controls. Feedback receives
keyboard focus and scrolls into view, with status/alert semantics. Product creation
and editing have distinct confirmations. Publication feedback also sits beside
its actions. Save buttons show progress while requests are pending.

Verification: Vue type checking and production build; isolated PostgreSQL-backed
browser checks for product creation/editing, focused visible success/error messages,
invalid price/alt text, unsaved-photo publication guard, inline initial publication
and photo republishing, fresh editor revision after publication, and the existing
missing-variant prerequisite. Synthetic fixture database, process, files and tab
were removed. No schema or backend changes; existing publication, ownership,
conflict and uncertain-save protections remain in use.

Go learning connection: the browser presents a faster route to the same Go
publication transaction. Revision checks in that transaction remain authoritative;
client-side disabled buttons alone cannot prevent a stale or conflicting write.

Next: basket with certificate owner-name collection and deterministic prices.
