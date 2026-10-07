# Public repository policy

Only source code, public documentation, synthetic fixtures and reviewed distributable assets belong here. Do not automatically synchronize a workspace, a merchant installation or the private planning pack into this repository.

## Before publication

Review the complete staged diff and file list. Exclude secrets, authentication tokens, payment credentials, MFA seeds and recovery codes. Exclude customer names, emails, addresses, orders, certificates, analytics exports and support conversations. Do not include private hostnames, personal filesystem paths, deployment identifiers or real operational logs. Screenshots and binary assets need manual review, including metadata and rights.

Keep runtime configuration outside Git. An example configuration may contain placeholders only. Use reserved domains such as example.com for synthetic fixtures. Git ignore rules prevent routine staging; the publication guard also rejects risky tracked file types and recognizable personal email/home-path patterns. Gitleaks detects supported secret patterns. None guarantees that all personal information or secrets will be found.

Local hooks reject checks that fail or cannot run. Enable them in each clone. CI checks public changes and history after a push; it cannot undo an exposure. GitHub push protection adds a pre-publication control for supported credential patterns. Maintainer review remains required.

## Public and private boundaries

Merchant commerce, admin, storefront, APIs, provider adapters, export and self-hosting tooling remain public. Subscription billing for our hosting customers and cloud fleet provisioning remain private initially. Public manifests must be generic and reference externally supplied secrets. The community product must run independently.

Keep planning notes containing founder circumstances, infrastructure inventory, commercial drafts or unreviewed research outside this checkout. Publish curated requirements and decisions instead. Customer/shop credentials never belong in ordinary data exports.

## Incident response

Stop publication and rotate exposed credentials before further work. Record only redacted evidence privately. Assess history, Actions logs, artifacts, issues and PRs; notify affected parties through the appropriate incident process. Removing Git history alone does not establish containment.
