# Container packaging and deployment status

The Dockerfile builds the Vue admin and static Go binaries in separate pinned
Node/Go stages. The scratch runtime includes the admin assets, CA certificates,
timezone database and licence notices. It runs as UID/GID 65532, with no shell or
package manager. Application startup never runs schema migrations automatically.

The build context is allowlisted in .dockerignore and explicitly excludes local
configuration, credentials, node_modules and dist. Build images from a reviewed
checkout; do not add merchant files to included source directories. No secrets
are build arguments or frontend environment inputs. Builder stages retain source,
but the runtime copies only binaries/assets/notices. Review image contents before
publication; neither a container build nor scans guarantee absence of sensitive data.

```sh
docker build --platform linux/amd64 --tag selloovy:container-check .
sh scripts/container-smoke.sh selloovy:container-check
```

CI builds and starts this image without a database or merchant credentials. It
checks non-root/read-only execution, liveness, public/admin pages and bundled assets, and refusal to
claim database readiness. This is packaging evidence, not commerce acceptance.
The image also includes /app/migrate, /app/owner-setup, /app/auth-keygen and
/app/square-sandbox-check; override the entrypoint for operator commands.

Cluster deployment still needs explicit configuration: HTTPS public origin,
private database URL with verify-full TLS outside loopback, authentication key,
database migrations/runtime roles, backup/restore and probes. Never expose the
unconfigured preview as a functioning merchant shop. Keep initial deployments
private until authentication/ingress and commerce acceptance gates pass.

Kubernetes projected Secret files are symlinks, while Selloovy's key loader rejects
symlinks and group/world permissions. Provision a private regular key file owned
by UID 65532 before starting the app, using a controlled init step and memory-backed
volume. Do not loosen the loader. Preserve the same installation key across
replicas/redeployments; Secret changes need a deliberate restart/rotation procedure.

Node-local storage is suitable for a controlled test deployment only when its
single-node failure and backup/restore behavior are understood. No replicated
storage, HA database, registry publication, cluster deployment or public launch is
claimed by this increment. Wallet/card policy and Square evidence are unchanged.

The image now includes `/app/worker`. Run it separately with the same private
database/installation/Square sandbox configuration for bounded source cleanup,
budget cleanup and leased GET-only payment reconciliation. It never runs migrations
or sends email automatically. See square-checkout.md for current sandbox status.
