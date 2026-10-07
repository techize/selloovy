# Go lesson 03: Go serves Vue's build output

Vue runs in the browser. Vite converts the Vue/TypeScript source into HTML, JavaScript and CSS. The Go server reads those built files; it does not run Node or TypeScript at request time.

**A filesystem is another dependency.** `web.NewHandler` accepts `fs.FS`, an interface for reading files. Production passes `os.DirFS(cfg.AdminDir)` for the trusted build directory. Tests use `fstest.MapFS` containing synthetic files, so route tests need no Node installation or real merchant data.

**A small route surface.** `/admin/` reads one fixed index file. `/admin/assets/*` permits only regular files inside the asset directory. File paths are validated and directories are rejected. Other paths, such as source files and private configuration, are not served by these routes. The filesystem root must remain a trusted, operator-owned build output directory, separate from uploads.

**The browser asks Go for state.** The Vue connection card fetches `/health/ready` on the same origin. It renders only the readiness status. This is not a privileged API, and a visible page does not give permission to access merchant data. Authentication and authorization must be implemented before merchant operations.

Optional exercise: read TestAdminAssetBoundary in internal/web/web_test.go. Explain why a file existing in the supplied filesystem does not automatically make it available over HTTP. Then identify the two separate checks that reject directory listing and parent-path traversal.
