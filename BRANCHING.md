# Branching strategy

Three long-lived branches, each one an environment. Code moves up exactly one
tier at a time, and never down.

```
  feature/*  ──PR──▶  develop  ──PR──▶  staging  ──PR──▶  main
             dev        pre-prod           prod
```

| Branch | Environment | Channel | Who consumes it |
|---|---|---|---|
| `develop` | integration | `dev` | you, on every push. `./install.sh --channel dev` |
| `staging` | pre-production | — | QA, integration checks |
| `main` | production | `stable` | everyone. `./install.sh --channel stable` |

`develop` is the only branch you push to directly. `staging` and `main` are
protected: they accept pull requests only, and only from the branch directly
below them.

## Why promotion is a pull request and not a merge

A promotion PR is the audit record. It shows exactly which commits reached
production, it runs the full suite against the target branch before anything
lands, and it leaves a revert path — revert the merge commit on `main` and the
Promote workflow carries the revert forward on the next release.

Merging `develop` straight into `main` would make it impossible to tell whether
anything was ever validated in a pre-production environment, which is the one
question a release is supposed to answer.

## Cutting a release

Promotions are one click each:

```bash
# develop -> staging (pre-production)
gh workflow run promote.yml -f target=staging

# staging -> main, publishes the stable channel
gh workflow run promote.yml -f target=production -f version=0.0.2
```

The workflow refuses to skip a tier. `develop` cannot reach `main` directly, and
`production` additionally requires a stable `MAJOR.MINOR.PATCH` version, because
the stable channel carries no prerelease suffix.

Once the production PR merges, tag it:

```bash
git checkout main && git pull
git tag v0.0.2 && git push origin v0.0.2
```

That tag drives the Release workflow, which publishes binaries, the SBOM and
the provenance attestation, then repoints `stable` in the `dist` channel
manifest.

### Tag provenance is enforced, not trusted

`Release` checks the tagged commit is reachable from the branch that owns the
channel, and refuses otherwise:

| Tag | Must be reachable from |
|---|---|
| `v1.2.3` | `main` |
| `v0.0.0-dev.123` | `develop` |

Release candidates are retired: there are no `-rc.N` tags. A candidate is
validated on `staging` (CI plus the release drills) and then tagged stable
from `main`; `release.yml` refuses prerelease tags other than `-dev.N`.

Tagging production from `develop` by hand fails the run. That is deliberate: the
branch policy is the thing that makes the three tiers mean something.

## Working on a change

```bash
git checkout develop && git pull
git checkout -b feat/short-description
# ... work ...
git push -u origin feat/short-description
gh pr create --base develop
```

Feature branches are throwaway. Rebase or merge `develop` into them as needed and
delete them once the PR lands. Nothing long-lived forks from `main`.

## Fixing something in production

Hotfixes go the same way round as everything else, with one extra step. Branch
from `main`, fix, then promote *back down* so the fix is not lost at the next
`develop` -> `staging` promotion:

```bash
git checkout main && git pull
git checkout -b fix/the-thing
# ... fix, add a regression test ...
git push -u origin fix/the-thing
gh pr create --base main --title "fix: the thing"
```

Then merge `main` into `develop` and `develop` into `staging`, so the fix is
present on every branch and the next promotion does not revert it.

```bash
git checkout develop && git merge main && git push
git checkout staging && git merge develop && git push
```

Doing this promptly matters: a fix that sits on `main` alone is one promotion
away from being silently undone.

## CI, and where it runs

CI runs on every push and pull request targeting `develop`, `staging` or `main`.

| Workflow | Runs on | Notes |
|---|---|---|
| `ci.yml` | GitHub-hosted | build, vet, format, test, cross-compile + artifacts, installer self-test |
| `security.yml` | GitHub-hosted | CodeQL analysis and govulncheck |
| `package.yml` | GitHub-hosted | `.deb`/`.rpm`/Arch and Homebrew formula |
| `release.yml` | GitHub-hosted | tag-driven: cross-compile, checksums, SBOM, attestation |

### Why race is its own job

The race detector slows the suite ~3x, so `go test -race` runs in parallel
with cross-compile and installer instead of inside the main test job — its
slowness stays off the critical path.

The cross-compile smoke test runs on the `linux/amd64` leg, which executes
natively on the hosted runner; the arm64 and darwin legs are compile-only.

## Branch protection

`main`, `staging` and `develop` are protected. `main` and `staging` require a
pull request, a passing `CI / Build & Test`, a passing `CI / Test (race)`, and
allow no force-push and no branch deletion. `develop` allows direct pushes,
because that is where day-to-day work lands, but still blocks force-push and
deletion.

The promotion workflow waits for those checks explicitly, merges with a merge
commit (no `--auto`, no squash — history stays append-only across tiers), and
then dispatches `ci.yml` on the destination branch: a merge made with
`GITHUB_TOKEN` triggers no push workflows by itself.

Two things sit on top of the checks for bot-authored promotions: an admin must
**approve** the pull request and every review thread must be **resolved**
(CodeQL comments count), or the Promote run's merge step fails. Approve the
bot's promotion PR promptly once its gates are green.

## Housekeeping

Deleting a branch on `main` would strand its commits, so protection forbids it.
Once a release is out and the branches have moved on, the old promotion PRs and
branches can be pruned with:

```bash
gh pr list --base main --state closed --limit 50
```
