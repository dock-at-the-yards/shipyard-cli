# Shipyard CLI ⚓

<p align="center">
  <strong>The <code>shipyard</code> command-line interface for building, running, and scoring evaluations of autonomous AI agents.</strong>
</p>

<p align="center">
  <a href="https://github.com/dock-at-the-yards/shipyard-cli/actions"><img src="https://img.shields.io/badge/build-passing-brightgreen.svg" alt="Build Status"></a>
  <a href="https://golang.org"><img src="https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go" alt="Go Version"></a>
  <a href="https://github.com/dock-at-the-yards/shipyard-cli/blob/main/LICENSE"><img src="https://img.shields.io/badge/License-Shipyard%20(source--available)-blue.svg" alt="License: Shipyard source-available"></a>
</p>

---

## Repository Role

This repository is the **command-line binary**: the `shipyard` binary, its
commands, and the release pipeline.

| Repository | Visibility | Role |
| :--- | :--- | :--- |
| **`shipyard-cli`** (this repo) | public | The `shipyard` binary, commands, and release pipeline. |
| [`shipyard-ci`](https://github.com/dock-at-the-yards/shipyard-ci) | public | Reusable CI templates that gate evals in GitHub Actions, Argo, Jenkins, and GitLab. |

---

## Requirements

- macOS or Linux, amd64 or arm64
- Docker, for `docker` sandbox environments (use `--env local` to run without it)
- `python3`, because rubrics are executed as `python3 <script>`
- `git`, since evalsets may reference remote agent repositories

**There is no Go requirement to install Shipyard.** You install a prebuilt
binary. See [Why you cannot `go install` this](#why-you-cannot-go-install-this).

---

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/dock-at-the-yards/shipyard-cli/main/install.sh | sh
```

That installs the latest **stable** release, currently `v0.0.1`. The
installer verifies the SHA256 checksum before writing anything, and it only
needs `curl` and `tar`.

```bash
# follow the test channel while you evaluate a new CLI
curl -fsSL https://raw.githubusercontent.com/dock-at-the-yards/shipyard-cli/main/install.sh \
  | sh -s -- --channel test

# pin an exact version, which is what you want in CI
curl -fsSL https://raw.githubusercontent.com/dock-at-the-yards/shipyard-cli/main/install.sh \
  | sh -s -- --version v0.0.1
```

| Channel | Tracks | Use for |
| :--- | :--- | :--- |
| `stable` | latest non-prerelease | production |
| `test` | latest `-alpha` / `-beta` / `-rc` | validating a release against your evals |
| `dev` | latest `-dev` build | debugging the CLI itself |

Or run it as a container, which is the easiest way to get Docker-in-Docker
sandboxing:

```bash
docker run --rm -it -v "$PWD:/src" -v /var/run/docker.sock:/var/run/docker.sock \
  ghcr.io/dock-at-the-yards/shipyard-cli:latest run ./my-eval
```

Verify, and uninstall:

```bash
shipyard version     # version, commit, channel
./install.sh --uninstall
```

### Distribution

Install with `install.sh`, which verifies a SHA256 checksum before writing
anything:

```bash
curl -fsSL https://raw.githubusercontent.com/dock-at-the-yards/shipyard-cli/main/install.sh | sh
```

`go install` is not supported. Use the checksummed release tarballs or the
container image.

---

## Quickstart

```bash
shipyard init eval fix-memory-leak
shipyard run ./fix-memory-leak
```

Inspect what ran:

```bash
shipyard version          # which build this is
cat fix-memory-leak/rollouts/*/metrics.json
```

Rollouts are written to `<eval-path>/rollouts/<run-id>/` containing
`trajectory.json`, `metrics.json`, `rubrics.json`, and `artifacts/`.

---

## Commands

| Command | Description |
| :--- | :--- |
| `shipyard init` | Scaffold an `eval` or `evalset` from a built-in template. |
| `shipyard run` | Execute an eval or evalset and score it. |
| `shipyard version` | Print the active version. |
| `shipyard completion` | Generate a shell completion script. |

### Useful flags for `shipyard run`

- `--agent <name>`: Override the agent driver.
- `--provider <name>`: Override the LLM provider (`openai`, `anthropic`, `gemini`, `ollama`, ...).
- `--model <name>`: Override the model name.
- `--command <cmd>`: Override agent execution command.
- `--env <type>`: Override the sandbox type (`docker`, `local`, `custom`).
- `--keep-env`: Retain the sandbox after completion for post-mortem debugging.

### Runtime overrides

Agents, models, providers, and sandbox environments can be swapped on the fly without editing config files.

---

## Development

```bash
make test      # run unit and integration tests
make build     # cross-compile (GOOS=linux GOARCH=amd64 make build)
make clean     # remove build artifacts
```

Version stamping is injected at link time and can be overridden at runtime with the `SHIPYARD_VERSION` environment variable.

---

## Releases

Shipyard publishes to three channels, and the tag alone decides which:

| Tag | Channel | Result |
| :--- | :--- | :--- |
| `v1.2.3` | `stable` | GitHub release + `:latest`, `:stable`, `:<version>` images |
| `v1.2.3-rc.1` | `test` | prerelease + `:test` image |
| `v0.0.0-dev.42` | `dev` | prerelease + `:dev` image |

Each channel is cut from its own branch, and the pipeline refuses to publish a
tag that is not reachable from it:

| Channel | Cut from | Reached by |
| :--- | :--- | :--- |
| `dev` | `develop` | pull request, or the automatic dev build on every push |
| `test` | `staging` | promoting `develop` to `staging` |
| `stable` | `main` | promoting `staging` to `main`, then tagging |

Code moves up one tier at a time and never skips a level, so nothing reaches
production without having been through a pre-production environment first. See
[BRANCHING.md](BRANCHING.md).

Every release cross-compiles for linux and darwin on amd64 and arm64, strips
symbols and local paths, smoke-tests the linux/amd64 binary, and publishes
SHA256 checksums, an SPDX SBOM, and a build-provenance attestation.

See [RELEASING.md](RELEASING.md) for the full runbook.

---

## License

**Proprietary — all rights reserved.** This repository is not open source and is not licensed under any open source license.

You may **not** copy, reproduce, redistribute, sublicense, publish, fork, mirror, or create derivative works of this software without prior written authorization. See [LICENSE](LICENSE) for the full terms.
