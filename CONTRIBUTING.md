# Contributing

## Development

Enter the Nix development shell and run the standard checks:

```console
nix develop
gofmt -w ./cmd ./internal
go test ./...
go vet ./...
go build ./cmd/actions-updater
```

Keep changes focused and add regression tests for behavior changes. Do not commit
the local binary or files under `dist/`.

## Releases

Push a tag matching `v*` to trigger the release workflow. It builds Linux,
macOS, and Windows artifacts and publishes checksums with the GitHub release.
