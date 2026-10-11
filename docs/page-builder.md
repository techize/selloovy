# Section page builder

In admin, open Pages. Create homepage uses the reserved `home` slug. Add an
information page with a unique lowercase slug such as `our-story`. Edit its title
and add hero, text/image, product collection and FAQ sections. Drag a handle onto
another section to reorder it, or use Move up / Move down. Pointer handles support
mouse and touch; the buttons provide the keyboard alternative and an announced
position change. Choose warm cream, soft sage or deep ink palettes.

Save draft gives explicit feedback beside the actions. Review saved preview then
Publish page or Republish page from the same editor. Unsaved changes must be saved
before review/publication. Page switching, reloading and leaving the workspace
prompt before discarding edits. Unpublish retains the draft; an unpublished draft
can be deleted. These are explicit owner actions, never automatic migration steps.

A published homepage appears at `/shop/<public-key>`. Information pages appear at
`/shop/<public-key>/pages/<slug>` and in the shop navigation. The complete catalogue
remains at `/shop/<public-key>/products`; existing `?after=` catalogue links still
work. Unpublishing the homepage restores the original catalogue landing page.
The installation's `/` foundation preview is unchanged. At least one published
product is required for the shop to be publicly available.

## Draft and publication boundaries

Owner-only GET/PUT `/api/admin/pages` uses existing session, origin and write-header
protection. A shared shop revision and locked transaction protect settings/page
changes from stale or concurrent writes. The page payload cannot select a shop.
Publication copies a saved draft to a separate snapshot. Later draft edits do not
change it. Schema 12 stores bounded draft/publication arrays on the shop, preserving
existing products, photos, authentication, shipping and baskets.

Content is structured plain text, rendered through Go html/template and Vue text
interpolation. There is no merchant HTML/JavaScript/CSS execution. Links are limited
to the catalogue and published information pages; a newly published information
page may link to itself. Three fixed palettes supply colour controls. Arbitrary
CSS, canvas positioning, themes, page-specific SEO controls and custom domains are
later work.

This first builder reuses published product cover photos and their alt text.
Independent page-image uploads are not yet available. Collections select 1–12
published products. The selector shows up to 500 published products and warns when
truncated. Publication rejects foreign or unpublished product references. Product
names, prices and covers follow their own product publications, independently of
page publication. Removing a product from publication removes its image/card from
public sections; a collection with no remaining products shows an unavailable
message. Admin offers removal of no-longer-published references before republishing.

Up to 20 pages and 20 sections per page, 12 FAQ entries, bounded Unicode text and
bounded total storage are enforced. These are POC limits, not a claim of unlimited
large-catalogue scaling. Preview presents the saved section order/content/palette
and currently loaded published product information; it is not an exact storefront
viewport simulator. Responsive CSS is included; mobile-width and real touch-device
verification remain outstanding. Payments, orders and stock holds remain separate
unfinished milestones.
