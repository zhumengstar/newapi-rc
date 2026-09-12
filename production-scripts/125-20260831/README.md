# Production scheduled-task snapshot

- Source host: `159.195.18.125`
- Collected at: `2026-08-31T14:37:37Z`
- Scope: active `newapi-*` and `account-manager-weekly-refresh` systemd service/timer/path units, their drop-in overrides, referenced `/usr/local/sbin/newapi-*` scripts, the account-manager weekly refresh script, and `/etc/newapi-peer-mesh.nodes`.
- Verification: all 75 copied files match the remote SHA-256 checksums (`CHECKSUM_DIFF` is empty).
- Sensitive runtime data intentionally excluded: environment files, credentials/tokens, private keys/certificates, logs, state directories, databases, Docker volumes, and historical backup files.

The files are stored under their original absolute-path layout (`etc/`, `usr/`, and `opt/`) so unit references remain easy to trace without installing anything locally.

Other production nodes checked during this run were not copied: `132.145.124.107` and `186.241.120.95` timed out during SSH banner exchange; `145.241.153.184` presented a changed host key and requires manual fingerprint verification before access.
