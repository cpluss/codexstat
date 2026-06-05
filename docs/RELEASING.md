# Releasing codexstat

The release workflow publishes two channels:

- pushing to `main` updates the moving `nightly` prerelease;
- pushing a `v*` tag publishes a stable GitHub release.

To publish a stable release:

```sh
go test ./...
git tag v0.4.4
git push origin v0.4.4
```

`.github/workflows/release.yml` builds macOS and Linux tarballs, publishes `checksums.txt`, and creates or updates the GitHub release.

After a stable release is published, verify the installer from outside the checkout:

```sh
tmp="$(mktemp -d)"
curl -fsSL https://raw.githubusercontent.com/cpluss/codexstat/main/install.sh | CODEXSTAT_INSTALL_DIR="$tmp" sh
"$tmp/codexstat" --json
```

The public Go install fallback depends on the module path in `go.mod`:

```txt
github.com/cpluss/codexstat
```
