# actions-updater

`actions-updater` scans GitHub Actions workflows and updates static GitHub
Action references to their newest Git tag while retaining the original YAML
formatting.

## Usage

```console
actions-updater [options] <workflow-file-or-directory...>
```

When a directory is supplied, the default scan target is
`.github/workflows/**/*.yml` and `.yaml`. Use `--recursive` to scan all YAML
files below the supplied directory. Explicit files are always processed.

```console
actions-updater .
actions-updater --dry-run .github/workflows/ci.yml
actions-updater --same-major --confirm .
```

Options:

- `--confirm`: write updates without an interactive prompt.
- `--dry-run`: show proposed updates only.
- `--json`: emit a JSON report. This is preview-only unless combined with
  `--confirm`.
- `--same-major`: select tags only from the current major version.
- `--include-prerelease`: consider pre-release tags.
- `--only owner/repo` and `--exclude owner/repo`: repeatable action filters;
  exclusion takes priority.
- `--recursive`: recursively scan YAML files.
- `--concurrency N`: maximum concurrent GitHub repository requests (default 8).
- `--timeout DURATION`: maximum duration for GitHub requests (default `5m`).
- `--backend gh|http`: query backend for this run, overriding the
  configuration file (see Configuration).
- `--version`: print build version information.

Set `GITHUB_TOKEN` to authenticate GitHub API requests and avoid anonymous
rate limits; when unset, the `gh_token` value from the configuration file is
used. The token is never printed.

Only static line-form `uses:` values are changed. Docker actions, local
actions, expressions, block scalars, and other ambiguous YAML constructs are
reported and skipped. Existing comments, indentation, quoting, line endings,
and key order remain unchanged.

The updater selects the highest stable semantic-version tag by default. It
falls back to tag timestamps only when a repository has no semantic-version
tags. `--same-major` needs a semantic-version current ref; otherwise the use is
skipped.

Exit codes are `0` for success, `1` for processing failures or timeout, `2` for
invalid arguments or inputs, and `130` if interrupted or if the user rejects
the confirmation prompt.

## Configuration

The first run creates the configuration file and prints its path (override the
location with `ACTIONS_UPDATER_CONFIG`; otherwise
`$XDG_CONFIG_HOME/actions-updater/config.json`, defaulting to
`~/.config/actions-updater/config.json`):

```json
{
  "gh_token": "",
  "backend": "gh"
}
```

- `gh_token`: persistent GitHub token, used when `GITHUB_TOKEN` is not set.
- `backend`: `gh` queries GitHub through the `gh` CLI (using its own
  authentication, falling back to `gh_token`), `http` calls the REST API
  directly. When the file is generated, `backend` is set to `gh` if the `gh`
  command is available on `PATH`, otherwise `http`. Pass `--backend` to
  override the stored value for a single run.

## Development

```console
nix develop
go test ./...
go build ./cmd/actions-updater
```

## Nix Installation

Install the latest tagged release directly from the flake:

```console
nix profile install github:ReCloudStudio/ActionsUpdater
```

Use it from another flake by adding the input and including the package:

```nix
{
  inputs.actions-updater.url = "github:ReCloudStudio/ActionsUpdater";

  outputs = { self, nixpkgs, actions-updater, ... }: {
    nixosConfigurations.example = nixpkgs.lib.nixosSystem {
      system = "x86_64-linux";
      modules = [
        ({ pkgs, ... }: {
          environment.systemPackages = [
            actions-updater.packages.x86_64-linux.au
          ];
        })
      ];
    };
  };
}
```

The package is also available as `actions-updater.packages.<system>.default`.

Build metadata can be injected with:

```console
go build -ldflags "-X main.version=v1.0.0 -X main.commit=$(git rev-parse --short HEAD) -X main.buildDate=$(date -u +%FT%TZ)" ./cmd/actions-updater
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for development and release notes.

## License

AGPL-3.0-or-later. See [LICENSE](LICENSE).
