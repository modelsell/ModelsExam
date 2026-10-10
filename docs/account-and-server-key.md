# Accounts, saved keys, retests and scheduled checks

Status: shipped 2026-10-09. Records became account-only on 2026-10-10.

Checks need no account. "My check records", saved keys, retests and schedules do: the
record list holds only the runs an account started while signed in. Guest runs are not
attached to any account and are in no list; their report links keep working, and their
rows stay in the database. (The browser `mc_owner` cookie now only lets a guest edit the
remark of a run it started.)

## Accounts (`internal/auth`, `internal/server/accounts.go`)

- Username only (3–32 `[A-Za-z0-9_]`, case-insensitive unique) and a password. No email,
  no 2FA, no self-service recovery.
- Password: at least 8 characters with at least one letter and one digit (relaxed from
  10 characters / 3 of 4 classes on 2026-10-10), at most 72 bytes (bcrypt), and not a
  common password. Existing passwords are unaffected. The
  embedded list (`internal/auth/weak-passwords.txt`) is matched directly, after undoing
  leetspeak and after stripping the digits and symbols people append (`P@ssw0rd2024`).
  Hash: bcrypt cost 12.
- Brute force: 5 consecutive failures lock the account for 15 minutes, each further
  failure doubles the lock up to 24 hours; a success resets it. Per IP: 10 login attempts
  a minute and 50 an hour (429); 3 registrations an hour. Unknown user and wrong password
  give the same "Incorrect username or password" (a dummy bcrypt comparison keeps the
  timing equal). Attempts are kept in `login_attempts` for 30 days.
- Sessions: 32 random bytes in the `mc_session` cookie (HttpOnly, SameSite=Lax, Secure on
  HTTPS, 7 days); only the SHA-256 is stored in `sessions`. Logout deletes the session; a
  password change needs the old password and ends every other session.
- State-changing account requests must be same-site (`Origin`, else `Sec-Fetch-Site`).
- Forgotten password: an operator runs `modelsexam user reset-password <name>`
  (in Docker: `docker exec modelsexam /modelsexam user reset-password <name>`). It prints a
  random password, ends all sessions and deletes all of that user's saved keys and
  schedules.
- "My check records" lists the account's runs (`model_check_runs.user_id`, latest 1000)
  from any device; `GET /api/model_check/history` answers 401 without an account. Guest
  runs are never moved into an account (the earlier claim flow was removed; its
  `owner_claims` table is kept unused because schema changes are add-only).

## HTTPS gate

`AUTH_REQUIRE_HTTPS` (default `true`) refuses register, login, password change and saving a
key unless the request is HTTPS. `X-Forwarded-Proto` is believed only from
`TRUSTED_PROXIES`. Production: Cloudflare → Caddy on the host → `127.0.0.1:8088` → the
container, which sees Caddy as `172.17.0.1`, so deploy sets `TRUSTED_PROXIES=172.17.0.1/32`.
With `MODEL_CHECK_CLOUDFLARE=true` the client address is taken from `CF-Connecting-IP`, but
only when the address Caddy reports is a Cloudflare edge range (a direct hit on the origin
cannot spoof it). Per-IP limits, including the existing "one check per address", use it.

## Saved keys (`credentials`)

Product decision: keys are stored **in plaintext** so the server can run checks without
the user. Mitigations:

- Write-only. `Secret` has no JSON name; list and detail queries select every column but
  `secret`; only `Store.CredentialSecret` reads it, for the check runner. The UI shows the
  mask (`sk-ab••••wxyz`) only. Reports, stream events and errors keep the existing
  redaction.
- Bound to a base URL. The base URL is normalized per protocol when saved; a check that
  names another base URL is refused. Using another address means entering the key again.
- Expiry is required: 1, 7 (default), 30 or 90 days; renewal sets up to 90 days from now.
  An hourly job hard-deletes expired keys and their schedules. The UI warns 3 days before
  expiry and after 14 days without use.
- Three consecutive upstream 401/403 answers from the key's host pause the key (counted
  across runs; the run stops sending once paused). The user resumes it on My keys.
- Limits: 20 keys per account, 50 checks per key per UTC day, one running check per key.
- Saving needs the "保存到我的账号" switch plus all three acknowledgements; the warning
  text is in the check forms and on My keys.
- SQLite file (and WAL/SHM) is chmod 600; deploy backups are root-only.
- Incident response: `modelsexam credentials purge-all --yes` deletes every key and schedule.

Image checks: the endpoint key is a credential of provider `image`; the OpenAI Verify key,
when used, is a separate credential of provider `openai` bound to `https://api.openai.com`
(`verify_credential_id`).

## One-click retest

"重新测试" on a record rebuilds the check (`web/src/features/account/lib/retest.ts`):
provider from the transport, base URL from the stored endpoint (older Claude reports kept
the full request URL; `/v1/messages` is stripped), model and options run through today's
form fields (unknown or invalid fields dropped, defaults filled, a Claude baseline that no
longer exists dropped, image provenance off). With a saved key for that provider and base
URL, a dialog shows host, model, suite, key mask and the request estimate and starts a
server-side run (`POST /api/jobs/retest`). Otherwise the home form opens filled in with the
cursor in the key field.

Each run stores a `config_key` (hash of transport, endpoint, model and non-default
options; backfilled on startup), and the records list shows the score change against the
previous run with the same key. Failures and score drops are highlighted.

## Server-side checks and schedules (`jobs.go`, `schedules.go`)

- Background runs use the same engines, history writer and redaction as the streaming
  endpoints, with their own slots (2 per provider, separate from the 2 manual ones), and
  keep running when the page is closed. On shutdown they are marked interrupted.
- `schedules`: one saved key + model + options; intervals 30m/1h/3h/6h/12h/24h; at most 10
  per account; next run = interval ±10%; the first run starts about a minute after
  creation. A 30-second loop claims due schedules with a compare-and-set on `next_run_at`,
  so a run never starts twice, and skips (retries next tick) when the key is busy or the
  slots are full. Before each run: key exists, not expired, not paused, under 50 checks
  today, SSRF validation passes. A missing, expired or paused key disables the schedule.
  Every attempt is recorded in `schedule_runs`.
- The schedules page shows running checks, the last result of each schedule and the
  estimate 「定时检测会持续使用这个 Key 并产生费用，预计每天 N 次请求。」.

## API

| Method | Path | |
|---|---|---|
| GET | `/api/auth/me` | `{user, https, require_https}` |
| POST | `/api/auth/register`, `/api/auth/login` | `{username, password}` |
| POST | `/api/auth/logout`, `/api/auth/password` | |
| GET/POST | `/api/credentials` | list (no secrets) / save `{provider, base_url, secret, expires_days, ack_test_key, ack_quota, ack_plaintext, name?}` |
| POST | `/api/credentials/:id/renew`, `/resume`; DELETE `/api/credentials/:id` | |
| GET/POST | `/api/schedules`; PATCH/DELETE `/api/schedules/:id`; GET `/api/schedules/:id/runs` | |
| POST | `/api/jobs/retest` | `{provider, credential_id, model, options}` → `{run_id}` |
| GET | `/api/jobs/running` | the account's running checks |

## Tests

`internal/auth` (password rules, tokens), `internal/store/accounts_test.go` (lockout,
session ending, write-only secret, expiry deletion, limits, schedule claim, account-only record lists,
previous score) and `internal/server/accounts_test.go` (HTTPS gate, cross-site refusal,
lockout and rate limits, logout and password change, records for accounts only, write-only API, base URL binding,
auto-pause, background retest, a schedule that runs once under concurrent ticks,
Cloudflare client IP). Frontend: `web/src/features/account/lib/*.test.ts`.
