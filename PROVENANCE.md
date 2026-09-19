# 4x-ui v4.0.1 provenance

This independent distribution is maintained from the GG Panel source snapshot
and does not overwrite the existing upstream repositories.

- Internal panel version: `4.0.1`.
- Panel build: GitHub Actions Ubuntu 24.04, Go 1.26.3, CGO enabled, static
  Linux amd64, trimpath.
- Xray: `26.4.25`, Linux amd64, copied from the verified `v4.0.0` runtime.
- Xray executable SHA256:
  `345781b0068ff2535360f90bb2eb20551950bb4f92bc7d44d2e6c693fa7dc210`.
- `v4.0.1` fixes unlimited per-client daily traffic limits being reset to the
  10 GB default by periodic traffic-statistics writes.

`scripts/package-release.sh` and the release workflow export a fixed allowlist
of program/runtime files. Production databases, client records, Xray runtime
configuration, TLS private keys, cloud credentials and backup credentials are
excluded.

This project retains the GPL-3.0 license and upstream attribution to
[3x-ui](https://github.com/MHSanaei/3x-ui). The Go module name and x-ui paths
remain unchanged for compatibility. Xray and bundled rule data retain their
respective upstream licenses; Xray's license is included in `bin/LICENSE`.
