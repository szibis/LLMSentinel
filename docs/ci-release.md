# Build and release checks

To recover a failed publication of an existing reviewed release, dispatch
`release.yml` from main with `version` and its immutable 40-character `ref`:

```sh
gh workflow run release.yml --repo szibis/LLMSentinel --ref main \
  -f version=v3.5.2 -f ref=04ce9012da88e2e1701d62ce9a828b7cdc772e63
```

The dispatcher verifies that the tag already exists and resolves to that commit,
the source VERSION matches, the commit remains on main, and its completed main
Build succeeded. It uses current workflow controls for the real Metal gate while
building release assets from the reviewed source. A branch name, a new tag, a
different tagged commit or failed Build is refused before hardware or publishing.
This retries that release's contents; newer fixes require a new reviewed release.

Build runs on main pushes and pull requests. Its required checks retain their existing names: four cross-platform builds, test, lint, security and docker-build. `llm-sentinel`, `sentinel-gateway` and `sentinel-tools` are built. Tests include the Go race suite and model-free Go lab/capture/controls/release/lifecycle tests. Lint uses golangci-lint v2.14.0 built with the project's Go version and validates workflow YAML with actionlint v1.7.12. Security uses govulncheck v1.8.0. The project and Docker builder use Go 1.27.2, including the [October security fixes](https://go.dev/doc/devel/release#go1.27.2).

Auto Release runs after a successful Build for a main push. Changes select a bump from merged PR titles: `feat:` selects minor, `feat!:` or a breaking-change title selects major, and `fix:`, `perf:` or `refactor:` selects patch. At the current main tip it prepares a reviewable release PR containing VERSION and changelog changes and dispatches a Build pinned to that preparation commit. Publication follows the reviewed release PR's merge and successful main Build. The reviewed commit may remain an ancestor after newer merges; publication still uses its exact tested SHA. Unsupported titles and commits without a merged PR skip release preparation. The old competing Auto-Tag workflow is removed.

The publisher receives the exact tested commit and version, builds all three binaries for Linux/macOS on amd64/arm64 (12 release assets), uploads distinct matrix artifacts and publishes release assets. GHCR image names are lowercase. Docker publishing is serialized, and only the newest semantic version tag can promote its image to latest. Rerunning a partially failed release reuses its commit tag and replaces assets rather than incrementing the version. An explicit pushed semantic version tag can also invoke Release; that path is intended for operator-selected releases.

Checks to run locally:

```sh
make lab-test
go test -race ./...
go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0 run ./... --timeout 5m
go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12 -shellcheck= -pyflakes=
go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...
docker build -t sentinel:check .
```

The Docker context excludes `.sentinel-lab`, client settings, generated binaries and model caches. Model weights and lab runtime state are never release assets. The Qwen Metal workflow checks real generation on an explicitly enabled Apple Silicon runner; see `qwen-metal-ci.md`. Run `make lab-model-check` to check an already running interactive lab.

## Manual publishing

Current releases include all three native binaries for four targets. Selecting
an immutable historical commit that predates `sentinel-tools` publishes that
commit's original two binaries; smoke controls still build from the trusted
publishing workflow's source. Historical source and tags are never rewritten.

`Publish Tested Release` provides an Actions button for operator-selected versions. Run it from main, entering a stable version such as `v3.1.0` and a ref such as `main`, an immutable main commit, or an existing release tag. The selector resolves the ref once and requires a successful completed Build for that exact commit on main. A new version must exceed existing stable tags. A retry can reuse its existing tag only when it still names the selected commit; release tags never move. The reusable publisher receives the immutable commit SHA, so later merges cannot change the published source.

```sh
gh workflow run publish.yml --repo szibis/LLMSentinel --ref main \
  -f version=v3.1.0 -f ref=main
```

The manual workflow shares the automatic release lock and calls the publisher directly after creating its tag. It does not depend on a workflow-token tag push creating another Actions run.

The older dependency PRs' failed security checks use an older Go standard library. After these fixes reach main, refresh those branches against main and rerun checks. PR #32 carries its own older workflows and substantial additional changes; its lint, security and frontend failures need evaluation on a refreshed branch rather than disabling required checks.
