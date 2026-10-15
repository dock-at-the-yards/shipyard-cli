# Releasing Shipyard

How versions move from a commit to a published artifact across the four
repositories. The short version:

```
feature ──PR──▶ develop ──PR──▶ staging ──PR──▶ main ──tag──▶ release
   branch        dev         pre-prod         prod    (prod artifact)
              (auto build)  (promote)      (promote)
```

Each arrow is a promotion, and each promotion moves exactly one tier. See
[BRANCHING.md](BRANCHING.md) for the full model.

## The two channels

A channel is a stream of versions. The CLI's `install.sh` can install from
either one.

| Channel | Branch | Tag shape | GitHub release |
| :--- | :--- | :--- | :--- |
| `dev` | `develop`, automatically on every push | `v0.0.0-dev.<run>` | prerelease, pruned to the newest |
| `stable` | `main` | `v0.0.2` | full release |

The tag alone decides the channel. The release workflow **refuses to publish**
any tag that is not reachable from the branch that owns its channel, so a
release cannot be cut from the wrong branch:

| Tag | Must be reachable from |
| :--- | :--- |
| `v0.0.2` | `main` |
| `v0.0.0-dev.42` | `develop` |

Release candidates are retired: there are no `-rc.N` releases. A candidate is
validated on `staging` (CI plus the release drills) and then tagged stable from
`main`; `release.yml` refuses prerelease tags other than `-dev.N`.

## Cutting a stable release

`staging` is what you are actually promoting, so it has to be green and have
everything you want to ship. In practice that means promoting to `staging` first
and letting it bake for a while.

Both hops go through the Promote workflow, which opens a pull request and merges
it once the required checks pass. It refuses to skip a tier, so `develop` cannot
reach `main` directly.

```bash
# 1. develop -> staging (pre-production)
git checkout develop && git pull
gh workflow run promote.yml --repo dock-at-the-yards/shipyard-cli -f target=staging

# 2. staging -> main. Pass the version, or leave it blank to use the VERSION
#    file on staging. Only stable MAJOR.MINOR.PATCH is accepted here.
gh workflow run promote.yml --repo dock-at-the-yards/shipyard-cli \
  -f target=production -f version=0.0.2

# add -f dry_run=true to open the PR without merging
```

Promoting to production requires that `staging` has already proven the change in
the pre-production environment. That is the entire point of the extra tier: time
on staging is the evidence that what is about to ship was actually exercised.

## Tagging the release

```bash
# tag the merge commit on main. This triggers the release pipeline.
git checkout main && git pull
git tag v0.0.2
git push origin v0.0.2
```

The pipeline then cross-compiles, smoke-tests, checksums, generates an SBOM,
attests provenance, publishes the GitHub release, and updates the `dist`
channel manifest.

## Cutting a dev build

Dev builds need no ceremony: every push to `develop` tags one automatically and
publishes a prerelease. To rebuild a specific commit by hand, tag it from
`develop`:

```bash
git checkout develop && git pull
git tag v0.0.0-dev.42
git push origin v0.0.0-dev.42
```

## Installing

```bash
# production
curl -fsSL https://raw.githubusercontent.com/dock-at-the-yards/shipyard-cli/main/install.sh | sh

# a development build, for debugging the CLI itself
curl -fsSL https://raw.githubusercontent.com/dock-at-the-yards/shipyard-cli/main/install.sh \
  | sh -s -- --channel dev

# pinned, for production
curl -fsSL https://raw.githubusercontent.com/dock-at-the-yards/shipyard-cli/main/install.sh \
  | sh -s -- --version v0.0.1
```

`shipyard version` reports the version, the CLI commit, the engine commit, and
the channel, so you can always tell what a given binary is.

## Engine versions

`shipyard-core` is private and is not distributed. It is tagged `core/v0.0.1`
when you want an immutable snapshot. Every released CLI binary records the
engine commit it was built against, so any published artifact traces back to
an exact engine state.

## Web

`shipyard-web` follows the same three tiers: pull requests are verified but never
published, `develop` is integration, `staging` is pre-production, `main` is
production. See [`deploy/README.md`](../shipyard-web/deploy/README.md).

## What is public

| Repository | Visibility | Why |
| :--- | :--- | :--- |
| `shipyard-cli` | public | the distributed binary and its source |
| `shipyard-ci` | public | the integration templates, useless without the CLI |

Distribution is through the checksummed release artifacts only. See `LICENSE`
for permitted use.
