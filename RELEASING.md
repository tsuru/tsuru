# Releasing

Releases are cut by [release-please](https://github.com/googleapis/release-please),
driven by [Conventional Commits](https://www.conventionalcommits.org/), following
the same setup as
[terraform-provider-tsuru](https://github.com/tsuru/terraform-provider-tsuru).

## How it works

1. Every push to `main` runs the `release-please` workflow, which keeps a single
   open pull request titled `chore(main): release <version>`. That PR holds the
   next version number, the generated `CHANGELOG.md` entry, and the bumped
   `api.Version` in `api/server.go`.
2. Merging that PR makes release-please push the `v<version>` tag and create a
   **draft** release whose body is the changelog entry.
3. The same workflow then dispatches `ci.yaml` on the new tag, which builds the
   `tsuru/api:<version>` image and runs goreleaser. goreleaser attaches the
   `tsurud` archives to the draft release and publishes it.

So the release only becomes visible once the tag has gone green through the
whole pipeline. No tokens beyond the default `GITHUB_TOKEN` are needed: tags
pushed with it do not trigger other workflows, which is why the tag is handed
over through `workflow_dispatch` instead of relying on the tag push.

Nothing has to be tagged or versioned by hand. The version reported by the API
comes from `api.Version`, which release-please keeps in sync with the tag.

## Commit messages

The version bump is derived from the commits landing on `main`:

- `fix: ...` (and `perf:`, `revert:`) bumps the patch number
- `feat: ...` bumps the minor number
- `feat!: ...` or a `BREAKING CHANGE:` footer bumps the major number
- anything else (`chore:`, `ci:`, `docs:`, unprefixed subjects) does not bump
  the version and is left out of the changelog

Pull requests are squash-merged with their title as the commit subject, so the
**pull request title** is what release-please reads. A release only happens once
at least one `fix:`/`feat:` commit has landed since the previous tag.

## The OpenAPI spec version

`docs/reference/api.yaml` carries its own `info.version` (`major.minor`), which
doubles as the SwaggerHub API version, so it deliberately does not track patch
releases. Bump it by hand with `make release version=<major.minor.patch>` when
cutting a new minor.
