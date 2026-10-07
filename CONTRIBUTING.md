# Contributing

Start with the current milestone and propose focused changes. Document the problem, resulting behaviour and verification. There is no contributor staffing or release schedule commitment yet.

Contributions are under Apache-2.0. Contributors retain copyright and certify their right to submit through the [Developer Certificate of Origin](https://developercertificate.org/). Sign commits using `git commit -s`. Review the rights and correctness of AI-assisted work yourself; generation is not proof of permission or quality.

Use a public Git identity you are comfortable publishing. Git commits and DCO sign-off become public records. Maintainers may use a verified GitHub noreply address; do not expose a private email address accidentally.

Enable hooks with `git config core.hooksPath .githooks`. Before each commit, inspect `git diff --cached` and run the checks in README.md. Never use real merchant or customer data in tests, screenshots, issue reports or pull requests. Use synthetic names and reserved example addresses. Report security issues privately through SECURITY.md.
