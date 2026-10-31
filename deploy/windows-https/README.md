# Windows HTTPS entry for a native U Workers deployment

This directory contains the exact files that front the native Windows
deployment with TLS and the local-admin login entry used for the CLAWSTA-80
desktop. They were verified on 2026-10-31 against the live deployment
(`D:\UWorkers\clawsta80\native`; P2 evidence in `evidence/clawsta-77/p2/`, P3
identity/lifecycle evidence in `evidence/clawsta-77/p3/`).

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
| `caddy-windows.ps1` | Lifecycle: `Start`/`Stop`/`Status`/`Run` plus CA `Trust`/`Untrust`/`ExportRoot`. Re-applies the ONSTART `UWorkers-CLAWSTA80-HTTPS` task definition on every `Start` (SYSTEM, no execution time limit, restart on failure, no battery stop), supervises `caddy run`, and stops orphans only by the exact managed executable path. |
| `Set-UWorkers-Login.ps1` + `.cmd` | Desk-host entry pinned to the identity in `<deployment>\local-login-identity.json`: it verifies the pinned user id, the address's SHA-256 fingerprint and the live database row, requires an HTTPS `PublicOrigin`, and then either opens `/login` (a credential already exists; never reset or overwritten) or runs `local-auth.exe bootstrap-host`, which mints a 15-minute single-use invitation and opens `/register#token=…` in a private window. Never asks for, stores, or prints a password or the invitation secret. `-Preflight` runs the checks with no side effect. |
| `create-local-identity.ps1` | One-time host-local admin task: idempotently ensures one identity exists (single transaction, mirrors the CreateUser fields, reuses an existing row without overwriting it) and optionally writes the pinned identity file. The app itself gains no signup or back door. |
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
6. Re-run `-Action Start` any time the script changes: it applies the task
   definition in place (including while the proxy is healthy) without
   restarting the supervisor.

## Identity and first use

1. Provision the one requested identity locally (admin, idempotent):

   ```powershell
   powershell -File create-local-identity.ps1 -Root <deployment> `
     -Email <identity-email> -DisplayName "<name>" `
     -IdentityFile <deployment>\local-login-identity.json
   ```

   Pin only the identity the requester explicitly supplied: compare the
   normalized (trim + lowercase, UTF-8) SHA-256 of the email with the supplied
   fingerprint. Never infer the identity from an older verification row, an old
   account with the same display name, or the runtime owner's profile (P3-R1
   corrected exactly that mistake).

2. The person signs in through the normal HTTPS entry once an entry credential
   exists and completes the normal first-use onboarding themselves (creates
   their own workspace, picks "skip the runtime for now"). Do not edit
   `onboarded_at` or create a workspace directly.
3. Run `Set-UWorkers-Login.cmd` from the desktop shortcut: it checks the pinned
   identity and the current release, then opens the web registration page. When no
   credential exists it mints a 15-minute single-use invitation over
   `local-auth.exe bootstrap-host` and opens `/register#token=…` privately; the
   person types their own login name and password in that page. When a credential
   already exists it opens `/login` and never touches the password. Afterwards
   everything — sign-in, invitations, user removal, password change and the
   instance open-registration switch — happens in Settings → 用户与注册.
4. Grant the instance administrator capability only to the explicitly pinned
   identity, never to a workspace owner by inference:

   ```powershell
   <deployment>\releases\<release>\local-auth.exe grant-admin --user-id <uuid> ^
     --deployment-config <deployment>\deployment.json
   ```

   It is idempotent and refuses nothing else: verify the UUID and the email
   fingerprint before running it.

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
