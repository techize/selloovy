# Foundation increment 03

Implemented: Vue/TypeScript admin preview, responsive workspace navigation, real readiness refresh, Vite build served through Go at /admin/, restricted asset routes, pinned Node/npm dependency lock and frontend CI type/build checks. No merchant records, settings writes, login or MFA are exposed.

Verified locally: Vue type checking and build; Go vet/race tests and build; database integration; asset route tests for missing builds, directories, source/private paths and traversal. Browser checks at desktop 1440px and mobile 390px verified layout and no mobile horizontal overflow. Keyboard Enter navigates to Products; Back to overview works. Actual database stop/restart produces not-ready/ready in the Vue status card. Browser warning/error logs are empty.

Compatibility: TypeScript 7 could not run the selected vue-tsc checker; the working toolchain pins TypeScript 5.9.3. Dependencies/notices are recorded; generated builds, node_modules and local credentials stay ignored.

M01-01 frontend toolchain/setup and M01-04 preview shells are implemented. The foundation build is usable, while Square sandbox enforcement evidence is still open. Next implement owner login/MFA before any privileged merchant API, then editable shop settings and the first product workflow. Full maker setup-to-test-order acceptance remains incomplete.
