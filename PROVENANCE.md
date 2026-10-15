# 4x-ui v4.0.7 provenance

This independent distribution is maintained from the GG Panel source snapshot
and does not overwrite the existing upstream repositories.

- Internal panel version: `4.0.7`.
- Panel build: GitHub Actions Ubuntu 24.04, Go 1.26.3, CGO enabled, static
  Linux amd64, trimpath.
- Xray: `26.4.25`, Linux amd64, copied from the verified `v4.0.0` runtime.
- Xray executable SHA256:
  `345781b0068ff2535360f90bb2eb20551950bb4f92bc7d44d2e6c693fa7dc210`.
- `v4.0.7` exports dated clients as `code|家宽| YY-MM-DD` or
  `code|高速| YY-MM-DD`, using the existing public lookup code and the client's
  SOCKS5 exit flag. Custom names remain supported. Quick creation adds a home
  exit toggle, available only with a valid inbound SOCKS5 exit, plus device
  presets 1, 2, 4, unlimited (0), and a custom integer from 0 to 9999.
  New forms default to the normal exit and one device. Existing customer
  credentials, stored names, expiry, limits and exit flags remain unchanged.
  The naming is shared by copied links, QR codes and all subscription formats.
- `v4.0.6` closes quick client creation after a successful save without waiting
  for the full client list refresh. List and online-status reads run concurrently;
  overlapping refreshes are coalesced, and saves request a post-mutation snapshot.
  Duplicate submissions are blocked while saving. Failed saves keep the form
  open; a failed background refresh warns that the client was already created.
  Client credentials, naming, expiry and device limits are unchanged.
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
