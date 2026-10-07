# Delivery roadmap

Work in small demonstrable milestones with a short resume note after each session. Explain Go concepts as features are implemented, with optional exercises. No fixed dates or weekly allocation are assumed.

1. **Foundation and payment proof:** runnable Go/Vue/PostgreSQL development setup and early Square sandbox investigation of strict default 3DS enforcement. See milestone-01.md. Live-account enforcement remains a separate gate.
2. **First visible shop:** owner password/MFA, editable settings, admin product creation, server-rendered listing and basket.
3. **Maker workflows and pages:** stocked/production variants, product-controlled fallback, personalisation, working-day estimates, shipping services and section drag-and-drop with draft/preview/publish.
4. **Complete test order:** provider sandbox, stock holds, deterministic totals, durable order snapshots, idempotent reconciliation, admin order view, customer/merchant notifications and abandoned-checkout administration.
5. **POC acceptance:** maker setup through a test order, persistence and isolated restore evidence. Record missing behaviour and collect feedback before expansion.
6. **Controlled live readiness:** verified live payment policy, security/accessibility, export/upgrade/restore, merchant obligations and operating ownership. Existing shop migration is a separate controlled activity.
7. **AI and cloud:** content/SEO/social drafts after the commerce POC, with explicit funding, budgets and approval. Paid cloud separately requires measured cost, isolation, service terms and recovery proof.

Payment proof may proceed alongside foundation work. A blocker must be documented and referred to the maintainer; do not silently relax 3DS or switch providers. Simulated payments are not a completed provider test order.

Defer manufacturing/material/base-cost tracking, general import, promotions, broad roles, channels and cloud automation beyond the first POC. Do not defer the selected section builder, basic stock, MFA, notifications or checkout recovery.

Cloud pilot direction: monthly billing without a minimum term; cancellation stops renewal, service continues through the paid period, then 90 calendar days of authenticated export-only access with selling disabled. Active store data is then deleted and backup copies expire under a disclosed schedule. Price, support hours, refunds, notices, exact retention and final terms remain to be established before paid launch.
