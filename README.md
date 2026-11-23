# my-tz-kori — integration repo

Builds and publishes the Komari Docker image from three independently versioned repositories.

| Repository | Contents |
| --- | --- |
| `ctfox66/my-tz-kori-server` | Go server (line `1.2.5-fix2`, no remote control / Web Shell) |
| `ctfox66/my-tz-kori-web` | default frontend theme embedded into the server binary |
| `ctfox66/my-tz-kori-agent` | Go agent, agent releases, install scripts |
| `ctfox66/my-tz-kori` | **this repo** — image build, Dockerfile, release wiring |

## Image

`ghcr.io/ctfox66/my-tz-kori`

## Publishing

Workflow `.github/workflows/release-docker.yml`:

- `workflow_dispatch` — inputs `version` (required), `server-ref`, `frontend-ref`,
  `publish-latest`.
- push of a tag matching `v*` — version is the tag without `v`.

It checks out the server repo, builds the theme from the frontend repo, injects it into
`server/web/public/defaultTheme`, cross-compiles both architectures, then builds the multi-arch
image. Tags: `<version>` and `sha-<short>`; `latest` is repointed only when requested (or on a
`v1.2.5-fix2` tag).

## Agent

Agent binaries and installer URLs are **not** handled here anymore: releases live in
`ctfox66/my-tz-kori-agent`, and the agent self-update points there too.

## Ink theme

`komari-theme-ink/` is a nested checkout of the Ink theme adaptation (own git remote), not part of
the image build.
