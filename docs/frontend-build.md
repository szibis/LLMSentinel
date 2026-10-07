# Frontend build

Use Node 20.19 or newer in the 20.x line, or Node 22.12 or newer. CI uses Node 24.
Install the locked dependencies and build from the repository root:

```sh
cd web
npm ci --no-audit --no-fund
npm run build
```

The Build workflow validates frontend sources alongside Go checks. Vite writes
generated files to `internal/service/static`; this check does not package those
files into Go or Docker releases. The native lab dashboard is served separately
by `sentinel-tools dashboard`.

Commit `web/package-lock.json` when changing dependencies so local and CI builds
use the same dependency graph.

Tailwind 4 runs through `@tailwindcss/postcss`. The stylesheet imports
`tailwindcss` and explicitly loads `tailwind.config.js` to retain the project's
theme and class-based dark mode. Responsive custom rules use standard CSS
media queries.
