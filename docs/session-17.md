# Increment 17: section page builder

Delivered owner page drafts, review/publish/republish/unpublish, homepage and
information-page navigation, four section types, colour palettes, published cover
selection, collections and FAQ editing. Reordering uses pointer capture with
move-up/down controls and position announcements. Saved feedback sits beside page
actions. Unsaved edits are guarded during page/workspace switching and reload.
The catalogue remains accessible, and homepage unpublishing restores its fallback.
Schema 12 starts with empty arrays and does not seed real merchant pages.

Owner-session/origin/header controls, shop scoping, shared-revision transactions,
validation, safe internal links and HTML escaping protect the draft/publication
boundary. Selected products must be owned and published. Product publications
remain independently controlled; unpublished products are omitted at render time.
Go race/PostgreSQL tests cover private drafts, information-page visibility,
publication/republishing, catalogue fallback, navigation, escaping, foreign product
rejection, stale changes, database rollback, concurrent changes and final-product
visibility. Vue types/build, Go vet/build/modules and publication scans are checked.

Isolated browser QA verifies hero/photo editing, collection selection, move
controls, pointer dragging, save feedback, saved preview, homepage publication,
text/image information-page creation, navigation, private edits until republish,
and FAQ disclosure. A synthetic screenshot is retained privately. No real merchant
page is created or published. Independent page media uploads, exact viewport
preview, mobile/touch-device proof and general theme customization remain open.

Next: stock holds, immutable order/checkout totals, Square strict 3DS proof,
confirmations and abandoned-basket administration. See page-builder.md and the
roadmap for the remaining POC/live gates.
