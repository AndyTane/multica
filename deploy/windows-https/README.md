# Windows HTTPS entry for a native U Workers deployment

This directory contains the exact files that front the native Windows
deployment with TLS and the local-admin login entry used for the CLAWSTA-80
desktop. They were verified on 2026-10-31 against the live deployment
(`D:\UWorkers\clawsta80\native`); see `evidence/clawsta-77/p2/`.

## Shape

```
LAN browser
   |  https://192.168.3.49:13443   (Caddy, LAN-only firewall rule)
   |      /api/* /auth/* /ws /uploads/* /v1/* /health* -> 127.0.0.1:18081 (Go API)
   |      everything else                              -> 127.0.0.1:13002 (Next.js)
   |  http://192.168.3.49:13001    (Caddy) GET/HEAD 308 to HTTPS, other methods 405
Caddy admin API: disabled. TLS: Caddy local CA, IP SAN for the LAN address.
```

The Next.js server moved from `0.0.0.0:13001` to `127.0.0.1:13002`; the Go API
and PostgreSQL stay loopback-only. Only the HTTPS entry and the plain-HTTP
redirect listener are reachable from the LAN.

## Files

| File | Purpose |
| --- | --- |
| `Caddyfile` | Reverse proxy config. Edit `192.168.3.49`, the ports and the two storage/log paths for another deployment. |
| `caddy-windows.ps1` | Lifecycle: `Start`/`Stop`/`Status`/`Run` plus CA `Trust`/`Untrust`/`ExportRoot`. Registers the ONSTART scheduled task `UWorkers-CLAWSTA80-HTTPS` (runs as SYSTEM) and supervises `caddy run`. |
| `Set-UWorkers-Login.ps1` + `.cmd` | Desk-host entry point for an operator/Tane to bind a password to an existing user with a hidden double prompt. Never stores or prints the password. |
| `create-login-shortcut.ps1` | Creates the public-desktop shortcut "设置 U Workers 登录". |
| `import-client-root.ps1` | Imports the exported public root CA into the current Windows user's trust store on a client machine. The one-time Windows confirmation dialog is confirmed programmatically only after the SHA-256 fingerprint check passes. |

## Install on a host

1. Download the pinned official Caddy Windows binary (v2.11.7 at the time of
   writing), verify the release `checksums.txt` SHA-512 and its Sigstore
   signature, and place `caddy.exe` in `<deployment root>\https`.
2. Copy `Caddyfile` and `caddy-windows.ps1` to the same directory; create
   `data\`, `logs\`, `state\`, `public\`.
3. Validate and start: `caddy.exe validate --config Caddyfile --adapter caddyfile`
   then `powershell -File caddy-windows.ps1 -Action Start`.
4. `-Action Trust` installs the CA root into the machine store and exports
   `public\root-ca.crt`. Copy only that public certificate to each client, then
   run `import-client-root.ps1` there.
5. Firewall: allow TCP `13443` inbound from the LAN subnet only; keep the
   existing `13001` redirect rule; the API/database ports must stay closed.

## Rollback

```powershell
powershell -File caddy-windows.ps1 -Action Stop          # stop the HTTPS entry
# Restore the previous release and HTTP-only listener, then start the deployment:
#   - set deployment.json Release back to the previous release directory
#   - set WebPort 13001, WebHost 0.0.0.0, PublicOrigin http://<lan-ip>:13001
#   - run runtime\native-windows.ps1 -Action Start
```

A pre-change backup (database dump, uploads, deployment.json with checksums)
lives in `<deployment root>\backups\native-*<`. CA `Untrust` removes the root
from the machine store; delete `https\data` only when intentionally retiring
the CA.

## Notes

- The admin API stays off; `Trust`/`Untrust` use `certutil` directly instead of
  `caddy trust`, which needs the admin API and fails in this Windows context.
- Caddy's console log format records method, URI, status and timing only; no
  request bodies, cookies or authorization headers are logged.
- The private CA key never leaves `https\data` (ACL: Administrators + SYSTEM).
- Client trust is per Windows user store. Every client machine needs the public
  root once; removal is `certutil -user -delstore Root <sha1-thumbprint>`.
