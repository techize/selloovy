FROM node:26.5.0-bookworm-slim@sha256:2d49d876e96237d76de412761cf05dbfe5aee325cc4406a4d41d5824c5bb8beb AS frontend
WORKDIR /build/web/admin
COPY web/admin/package.json web/admin/package-lock.json ./
RUN npm ci --ignore-scripts
COPY web/admin/ ./
RUN npm run typecheck && npm run build

FROM --platform=$BUILDPLATFORM golang:1.26.5-bookworm@sha256:53eeac89074db483fdf0ab3be1df32bf6e47562263d2d0d6baa7f26acb4957dd AS backend
ARG TARGETOS
ARG TARGETARCH
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download && go mod verify
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY db/migrations/ ./db/migrations/
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} go build -trimpath -o /out/ ./cmd/selloovy ./cmd/migrate ./cmd/owner-setup ./cmd/auth-keygen ./cmd/square-sandbox-check ./cmd/worker

FROM scratch
COPY --from=backend /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=backend /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=backend /out/ /app/
COPY --from=frontend /build/web/admin/dist/ /app/admin/
COPY LICENSE /app/LICENSE
COPY third_party/ /app/third_party/
USER 65532:65532
WORKDIR /app
ENV SELLOOVY_HTTP_ADDR=0.0.0.0:8080 SELLOOVY_ADMIN_DIR=/app/admin
EXPOSE 8080
ENTRYPOINT ["/app/selloovy"]
