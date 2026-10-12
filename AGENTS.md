# Working in Selloovy

This is a public repository. Follow docs/public-repository-policy.md before every commit or push. Never copy private planning packs, personal notes, home-directory paths, real customer/order data, credentials, logs, backups or production configuration here. Use synthetic examples under reserved example domains.

Keep original platform features and self-hosting tools open source. Cloud subscription billing and fleet provisioning stay outside this repository. Do not make community operation depend on those services.

Build with Go/Chi/PostgreSQL/pgx/sqlc, Vue/TypeScript admin and Go-rendered storefront. Respect docs/roadmap.md and the current milestone. Prioritize completing the working commerce platform and deployment. Continue implementation, meaningful verification and delivery without stopping after each increment; pause only when user information/access or a substantive decision is required. Do not add lessons or exercises unless requested again.

Before publishing, review staged files, run the publication guard and Gitleaks, use a public Git identity and DCO sign-off. Do not print detected secret values. Follow docs/payment-policy.md: Square determines required SCA under its standard policy; supply verification details and never override provider declines. Do not silently change providers or bypass MFA for owners who have enabled it. MFA is optional and recommended, including initial CLI setup; follow the current owner-authentication policy. Do not claim successful provider enforcement without evidence. Keep real deployment secrets outside Git. Include meaningful verification for payment, inventory, permission and recovery changes.
