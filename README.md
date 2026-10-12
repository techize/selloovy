# Selloovy

**Selling, smoothly.** An open-source ecommerce platform for makers and microbusinesses, launching in the UK.

Free to use and self-host for businesses of any size. An optional paid cloud service will operate the same merchant features, with managed hosting, updates, backups and bounded support.

## Project status

This repository is the home of the public platform source, development guidance and delivery backlog. Go storefront and Vue admin previews, PostgreSQL migrations and database readiness are available. Operator owner setup and browser login are available when configured. MFA is optional and recommended, with an admin reminder and QR setup; enabled MFA is enforced. Authenticated shop identity settings are editable. Private product drafts can be created, edited and listed, with variants, stock counts and product-controlled made-to-order fallback. Reviewed publish/republish/unpublish and public shop/product browsing are available. One reviewed JPEG/PNG cover photo per product and alt text are available. Guest baskets, certificate owner names and published-price subtotals are available. Editable UK shipping services, free-delivery thresholds and combined working-day estimates are available. A section page builder with private drafts, saved preview, drag reordering and explicit publication is available. Guest checkout details and reviewed totals are available, with durable financial attempts, inventory allocation, Square sandbox checkout/receipts and a recovery worker. Owner order/payment views, unfinished checkouts, verified reconciliation and whole-order dispatch/tracking are available. Confirmation jobs are queued; email delivery remains in progress. There is no production-ready release. The name remains subject to clearance.

Selected architecture: Go with Chi, PostgreSQL, pgx and sqlc; Vue with TypeScript for merchant admin; Go-rendered public storefront. Begin with a modular application and worker, extracting services only when measured needs justify it.

The first POC proves that a maker can configure a shop and complete a test order. It includes stocked and made-to-order goods, personalisation, a drag-and-drop section builder, optional recommended owner MFA and safe checkout recovery. Square is the payment provider; its standard SCA/risk decisions apply, with verification details supplied and application abuse controls. Product descriptions, SEO and social drafts with approval follow the commerce demonstration.

- [Container packaging and deployment status](docs/container-deployment.md)
- [Immutable payment preparations](docs/payment-preparations.md)
- [Go transaction and retry lesson](docs/go-learning-20.md)
- [Payment-token envelope](docs/payment-envelope.md)
- [Square sandbox adapter](docs/square-adapter.md)
- [Current Square payment policy](docs/payment-policy.md)
- [Square policy research and sandbox account check](docs/square-payment-proof.md)
- [Merchant orders and fulfilment](docs/order-management.md)
- [Square sandbox checkout and recovery worker](docs/square-checkout.md)
- [Persistent payment attempt limits](docs/payment-attempt-limits.md)
- [Durable financial lifecycle](docs/financial-lifecycle.md)
- [Payment boundary and remaining integration](docs/payment-attempt-boundary.md)
- [Go read-only HTTP client lesson](docs/go-learning-19.md)
- [Checkout review and allocation](docs/checkout.md)
- [Go checkout transaction lesson](docs/go-learning-18.md)
- [Section page builder](docs/page-builder.md)
- [Go page publication lesson](docs/go-learning-17.md)
- [Delivery roadmap](docs/roadmap.md)
- [First milestone](docs/milestone-01.md)
- [Local startup and verification](docs/development.md)
- [First Go lesson](docs/go-learning-01.md)
- [Go persistence lesson](docs/go-learning-02.md)
- [Go serves Vue lesson](docs/go-learning-03.md)
- [Go credential lesson](docs/go-learning-04.md)
- [Go transaction lesson](docs/go-learning-05.md)
- [Go browser authentication lesson](docs/go-learning-06.md)
- [Go optional-MFA lesson](docs/go-learning-07.md)
- [Go shop settings lesson](docs/go-learning-08.md)
- [Go product save lesson](docs/go-learning-09.md)
- [Go stock transaction lesson](docs/go-learning-10.md)
- [Go publication projection lesson](docs/go-learning-11.md)
- [Go immutable media lesson](docs/go-learning-12.md)
- [Product cover photos](docs/product-photos.md)
- [Public catalogue delivery](docs/storefront-publication.md)
- [Maker variants delivery](docs/maker-variants.md)
- [Product draft delivery](docs/product-drafts.md)
- [Shop settings delivery](docs/shop-settings.md)
- [Owner authentication delivery](docs/owner-authentication.md)
- [Public repository policy](docs/public-repository-policy.md)
- [Contributing](CONTRIBUTING.md)
- [Security reporting](SECURITY.md)

## Repository checks

Install Go and [Gitleaks](https://github.com/gitleaks/gitleaks), then enable the local hooks:

```sh
git config core.hooksPath .githooks
go run ./scripts/publicguard.go
gitleaks git --log-opts=--all --redact --no-banner --ignore-gitleaks-allow
```

Hooks are local safeguards and must be installed in each clone. GitHub Actions checks publication policy, scans Git history, verifies Go formatting/integrity/vet/race tests/builds and checks Vue types/builds. Review every staged change before publishing; automated checks cannot identify all personal information or credentials.

## Licence and boundaries

Original platform code is licensed under [Apache-2.0](LICENSE). Contributions use DCO sign-off and retain contributor copyright. The project is initially maintained by Techize. Trademark permission is separate from the software licence.

All merchant features, APIs and self-hosting tools belong here. Cloud-only subscription billing and fleet provisioning will remain in separate private repositories initially. A self-hosted shop must operate without those private services.
