# testdata/pikaid-v1.json

Verbatim copy of the official interoperability fixtures published by the
reference PHP implementation:

- Repository: https://github.com/pikaid/pikaid-php
- Tag: `v1.2.0`
- Commit: `e3f843edeb5157a5d2d25196cac0d69580810a95`
- Path: `tests/vectors/pikaid-v1.json`
- SHA-256: `b0c9dfdd063613424414fef5c9065fbd2f81d645d0ab42b34e27cde715613500`

The `pikaid-v1` filename is historical. The identifier format itself is
immutable and unversioned; these fixtures are used as an independent
interoperability check against specification revision 1.0.4, not as a
normative source. The canonical specification always wins.

The accompanying `pikaid-v1.schema.json` (also at the same tag and commit) is
not vendored: `vectors_test.go` asserts the structural invariants this
Go implementation relies on (field widths, hex/Base36 shapes) directly.
