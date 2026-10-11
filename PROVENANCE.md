# 4x-ui v4.0.5 provenance

This independent distribution is maintained from the GG Panel source snapshot
and does not overwrite the existing upstream repositories.

- Internal panel version: `4.0.5`.
- Panel build: GitHub Actions Ubuntu 24.04, Go 1.26.3, CGO enabled, static
  Linux amd64, trimpath.
- Xray: `26.4.25`, Linux amd64, copied from the verified `v4.0.0` runtime.
- Xray executable SHA256:
  `345781b0068ff2535360f90bb2eb20551950bb4f92bc7d44d2e6c693fa7dc210`.
- `v4.0.5` defaults `restartXrayOnClientDisable` to false in both the backend
  and frontend. Fresh installations and missing settings use the new default;
  explicitly stored true/false values are preserved. Existing installations
  that enabled the option must turn it off in panel settings to opt out.
  Normal auto-disable handling removes the affected user's access via the
  Xray API without forcing a global restart solely for that event. Existing
  connections may continue; API failure recovery can still require a restart.
  Client identities, quota values and daily reset times are unchanged.
- `v4.0.4` puts a public customer lookup code first in copied links and all
  subscription formats, followed by a compact expiry date. Custom names remain
  visible and searchable by their deterministic short code. Customer credentials,
  expiry enforcement and existing database records are unchanged.
- `v4.0.3` limits manual daily allowance choices to 10 GB, 20 GB, 30 GB and
  unlimited. Normal panel startup migrates existing 5 GB and 15 GB values
  upward to 10 GB and 20 GB before serving traffic.
- The `v4.0.1` fix that preserves unlimited per-client daily traffic limits
  during periodic traffic-statistics writes remains included.

`scripts/package-release.sh` and the release workflow export a fixed allowlist
of program/runtime files. Production databases, client records, Xray runtime
configuration, TLS private keys, cloud credentials and backup credentials are
excluded.

This project retains the GPL-3.0 license and upstream attribution to
[3x-ui](https://github.com/MHSanaei/3x-ui). The Go module name and x-ui paths
remain unchanged for compatibility. Xray and bundled rule data retain their
respective upstream licenses; Xray's license is included in `bin/LICENSE`.
