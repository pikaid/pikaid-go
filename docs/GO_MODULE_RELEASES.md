# Automated Go module discovery

Go modules are published by immutable Git tags, not by uploading a file to a
package registry. The module is `github.com/pikaid/pikaid-go`.

The workflow at `.github/workflows/publish-go.yml` runs when a stable
`vX.Y.Z` tag is pushed. It verifies the module identity, runs native tests
and Go vet, and asks the public Go proxy to fetch **that exact tagged
version**. This causes the version to enter Go's module index, which
`pkg.go.dev` monitors. There is no API key, registry upload, or manual
submission per release.

Go v2+ uses semantic import versioning; it normally requires `/v2` in the
module path. This workflow deliberately fails for v2+ tags until the module
path and import compatibility have been updated explicitly.

The Go proxy permanently records module versions; do not move tags or
overwrite previously published releases.

- Official process: https://go.dev/doc/modules/publishing
- Discovery: https://pkg.go.dev/about
- Go package: https://pkg.go.dev/github.com/pikaid/pikaid-go
