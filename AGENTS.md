# Working in Selloovy

This is a public repository. Follow docs/public-repository-policy.md before every commit or push. Never copy private planning packs, personal notes, home-directory paths, real customer/order data, credentials, logs, backups or production configuration here. Use synthetic examples under reserved example domains.

Keep original platform features and self-hosting tools open source. Cloud subscription billing and fleet provisioning stay outside this repository. Do not make community operation depend on those services.

Build with Go/Chi/PostgreSQL/pgx/sqlc, Vue/TypeScript admin and Go-rendered storefront. Respect docs/roadmap.md and the current milestone. Teach Go through feature work with short explanations and optional exercises; learning supports delivery.

Before publishing, review staged files, run the publication guard and Gitleaks, use a public Git identity and DCO sign-off. Do not print detected secret values. Do not weaken payment/MFA policies or claim successful provider enforcement without evidence. Keep real deployment secrets outside Git. Include meaningful verification for payment, inventory, permission and recovery changes.
