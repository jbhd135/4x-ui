# 4x-ui v4.0.0 provenance

This independent distribution preserves the GG Panel production version
verified on 2026-09-15. No existing upstream repository was overwritten.

- Internal panel version: `2.9.3-gg1` (intentionally unchanged).
- Panel executable SHA256: `7be86e4b03645b2e1cc0d557334d5faff4f4ec9931e214f78c77df95779b8050`.
- Xray: `26.4.25`, Linux amd64.
- Xray executable SHA256: `345781b0068ff2535360f90bb2eb20551950bb4f92bc7d44d2e6c693fa7dc210`.
- Production build: Go 1.26.3, CGO enabled, static Linux amd64, trimpath.
- Base source revision recorded by the executable:
  `e7b02ed54fc1c1e80d438f8ccb9477ad40c07762` with local changes.
- This repository includes the maintained source working tree with the GG
  client-level limits, expiry, subscription, SOCKS5 and maintenance fixes.
  Packaging and installation are maintained separately here.

The initial binary is copied byte-for-byte from the verified running panel,
not rebuilt from this new repository. A CI source build is provided as an
artifact and is not claimed to be byte-identical to the production build.

`scripts/package-release.sh` exports a fixed allowlist of program/runtime
files. Production databases, client records, Xray runtime configuration, TLS
private keys, cloud credentials and backup credentials are excluded.

This project retains the GPL-3.0 license and upstream attribution to
[3x-ui](https://github.com/MHSanaei/3x-ui). The Go module name and x-ui paths
remain unchanged for compatibility. Xray and bundled rule data retain their
respective upstream licenses; Xray's license is included in `bin/LICENSE`.
