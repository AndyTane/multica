# Login interface focused review (deployed password-only instance)

Reviewed on the live LAN HTTPS deployment `https://192.168.3.49:13443` from the
client laptop, with the browser trusting the exported root CA normally (no
`ignoreHTTPSErrors`/`-k`). Raw results: `browser-evidence.json` (18/18 steps
passed) and the screenshots in `browser/`.

## Passed

| Area | Result |
| --- | --- |
| Method set | Password-only server: the email-code form and resend flow are not reachable; `#login-email` never rendered. |
| Chinese copy | Title "登录 U Workers", labels 账号 / 密码, button 登录, all rendered on the live page. |
| Masking and autocomplete | account input `autocomplete=username`; password `type=password`, `autocomplete=current-password`. |
| Keyboard | Tab order account → password → submit; submitting with Enter on the password field works. |
| Duplicate submit | Submit button is disabled while the login request is in flight (delayed-response check). |
| Failure and retry | Wrong password shows an error, keeps the username, and the form can be retried without reloading. |
| Post-login navigation | Successful login lands on the protected workspace board `/c1-cross-machine/issues`. |
| Refresh | Reload keeps the session and the same protected page. |
| Logout | Session teardown returns 200 and `/api/me` is 401 afterwards; signing in again returns to the board. |
| Session cookie | `multica_auth`: HttpOnly, Secure, SameSite=Strict on the HTTPS origin. |
| Realtime | The app opened `wss://192.168.3.49:13443/ws?...` through the proxy after login. |

## Findings (non-blocking, not changed in P2)

1. **Config-resolution flash.** While `/api/config` resolves, the page first
   renders the email-code form and then switches to the password form. Measured
   on the live page: email form at ~102 ms, password form at ~303 ms after
   document start. Recommendation: expose a "config loaded" flag from the config
   store and render a neutral loading state on the login page until it is set,
   so a password-only server never shows the wrong method for ~200 ms. This is a
   small shared-config change, not a redesign.
2. **Server message language.** A failed login currently displays the API's
   English sentence ("invalid username or password") in the otherwise Chinese
   UI, even though the client already has `errors.invalid_credentials`.
   Recommendation: map HTTP 401/429 on the password route to the translated
   copy (and show a "too many attempts" message for 429) instead of printing
   `err.message` verbatim.
3. **Password retention on failure.** A failed attempt keeps both the username
   (intended) and the typed password in the field. Keeping the username is
   correct; consider clearing the password so a retry is an explicit re-entry.

None of these block the trial login; they are UX polish for the next pass.
