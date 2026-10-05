# ModelsExam

> Give any model API an exam. Open-source, independent conformance and authenticity tests for Anthropic Claude, OpenAI-compatible and image model endpoints.

[![CI](https://github.com/modelsell/ModelsExam/actions/workflows/ci.yml/badge.svg)](https://github.com/modelsell/ModelsExam/actions/workflows/ci.yml)

Live demo: [modelsexam.com](https://modelsexam.com/)

Standalone model-authenticity and protocol-conformance checker for **Anthropic Claude**
(`/v1/messages`) and **OpenAI-compatible** (`/v1/chat/completions`, `/v1/responses`, …) endpoints.
Extracted from the `new-api` fork; the check engines (`pkg/claudecheck`, `pkg/openaicheck`) are
unchanged, the platform glue (users, channels, relay adaptors) is gone.

- **Backend:** Go (gin + gorm; SQLite by default, PostgreSQL/MySQL via `SQL_DSN`)
- **Frontend:** React 19 + TanStack Query + Tailwind v4, built with Rsbuild, embedded into the Go binary
- **No accounts.** Anyone can run a check with their own Base URL and API key. The key is used for
  that one run, redacted from every report/stream/DB row, and never stored.

## Run

Requires Go 1.25.1 or newer and Bun 1.3.14 or newer. Frontend tests also require Node.js 24.

```bash
git clone https://github.com/modelsell/ModelsExam.git
cd ModelsExam
go mod download
make build                  # bun install + build web + go build  -> ./modelsexam
./modelsexam               # http://localhost:8080
```

Development: `make dev-api` (API on :8080, private upstreams allowed) and, in another terminal,
`cd web && bun run dev` (serves on :5178, override with `WEB_PORT`; proxies `/api` to :8080, override with `API_URL`).

Docker:

```bash
docker build -t modelsexam .
docker run --rm -p 8080:8080 -v modelsexam-data:/data modelsexam
```

Validate a source checkout with `make build && make test && go vet ./...`.

| Env | Default | Meaning |
|---|---|---|
| `LISTEN_ADDR` | `:8080` | listen address |
| `SQL_DSN` | `data/model-check.db` | SQLite path, `postgres://…`, or `mysql://user:pw@tcp(host:3306)/db?parseTime=true` |
| `MODEL_CHECK_ALLOW_PRIVATE` | `false` | allow loopback/private upstreams (local dev only) |
| `MODEL_CHECK_VERIFY_BASE_URL` | `https://api.openai.com` | OpenAI Verify host for image provenance checks (override only for tests) |
| `MODEL_CHECK_SITE_URL` | derived from the request | public origin, e.g. `https://modelsexam.com`; used for canonical links, `robots.txt` and `sitemap.xml`. Set it in production |
| `TRUSTED_PROXIES` | none | comma-separated CIDRs allowed to set `X-Forwarded-For` (needed behind a reverse proxy for per-IP limits) |

## API

All under `/api`, JSON, `Cache-Control: no-store`.

| Method | Path | |
|---|---|---|
| POST | `/model_check` | Claude check (`{base_url,key,model,…options,remark?}`); `Accept: text/event-stream` streams progress |
| POST | `/model_check/openai` | OpenAI check, same shape |
| POST | `/model_check/image` | image check (`{base_url,key,model,suite,provenance?,baseline?,verify_key?,remark?}`); `verify_key` is an official OpenAI key sent only to the Verify host |
| GET | `/model_check/history` | this browser's "My check records" list (owner cookie), **latest 100 only** (`page`, `page_size`, `model`, `status`); older rows stay in the DB but are never listed |
| GET | `/model_check/history/:id` | report detail (+`markdown` for OpenAI and image); 404 unless the owner cookie matches |
| PATCH | `/model_check/history/:id/remark` | only the browser that ran the check (owner cookie) |
| GET/POST | `/model_check/baselines` | shared comparison baselines (Claude) |

## Behaviour you should know about

- **Private history.** A report is listed and served only to the browser that ran it, identified by a
  random `mc_owner` cookie (no accounts). Clearing cookies or switching browsers loses access; the rows
  stay in the database. There are no public record lists, model boards, stats or report sitemap
  entries. Site badges still show the verdict (tier, score, date) of the newest check of a domain, from
  any visitor, but never link to the report.
- **Shared baselines.** Any visitor can promote a completed Claude report they ran to a baseline, and baselines
  are used for everyone's comparisons, so they can be polluted. Put the service behind auth or a
  reverse-proxy allow-list if that matters, or remove `POST /baselines`.
- **No cross-user isolation** beyond the owner cookie (a random UUID, not authentication).
- **Abuse limits:** one running check per client IP per provider, two concurrent checks per provider
  globally. There is no request-rate limiter; add one at the proxy for a public deployment.
- **SSRF:** every dial is validated after DNS resolution (private/loopback/link-local/CGNAT/metadata
  ranges blocked), no proxies, no redirects (`internal/ssrf`).
- **Dropped vs. new-api:** checks of configured channels and native AWS Bedrock (SigV4) checks.
  Only user-supplied endpoints are supported (Bedrock via an Anthropic-compatible gateway still works).

## Layout

```
cmd/model-check      main
internal/server      HTTP handlers, SSE streaming, history persistence
internal/store       gorm models (runs, baselines)
internal/ssrf        outbound client with dial-time IP validation
pkg/claudecheck      Claude probes, scoring, baselines   (from new-api)
pkg/openaicheck      OpenAI probes, scoring, markdown    (from new-api)
pkg/imagecheck       image generation/edit probes, local pixel verification, provenance stage
pkg/media            stdlib-only image inspection and synthetic fixtures
pkg/provenance       OpenAI Verify (content_provenance_checks) client and verdict rules
web/                 React app; web/dist is embedded by web/embed.go
docs/                protocol conformance + design notes. These were written inside new-api: paths such as
                     `controller/model_check_*.go` now live in `internal/server/`, and channel-based checks no longer exist.
                     `docs/removal-from-new-api.md` lists what to delete from new-api.
```

## License

Derived from [new-api](https://github.com/QuantumNous/new-api) (GNU AGPL-3.0, see `LICENSE`); the
frontend files keep their original headers. Running a modified copy as a network service has AGPL
source-offer obligations.

## Open source

ModelsExam is licensed under the **GNU AGPL-3.0** (see `LICENSE`). If you run a modified version as a
network service you must offer its source to your users. See `NOTICE` for trademark non-affiliation and
upstream attribution, `CONTRIBUTING.md` to contribute and `SECURITY.md` to report a vulnerability.

## Site content you edit by hand

- `web/src/config/baselines.ts`: official baselines. Run the exam on the official endpoint, then put the public report id in `reportId`. Rows without one read "Reference run pending". Never invent a result.
- `web/src/config/site.ts`: `REPO_URL`, the public repository link shown in the footer.

## SEO

Pages have real URLs (`/`, `/baselines`, `/records`, `/method`, `/reports/<id>`). The Go server renders each page's title, description, canonical link, Open Graph tags and a plain-HTML fallback into `index.html` (`internal/seo`), serves `/robots.txt` and `/sitemap.xml`, and answers unknown paths with 404. Each report has its own page (`/reports/<id>`, also the share link). When a check starts, the server reads the tested site's home page (no credentials sent, SSRF-checked, size and time limited) and stores its name (`og:site_name` or `<title>`) and description (`meta description`). A completed report with a site name is indexable: that name and description become its title and summary, and it is listed in `sitemap.xml` (latest 1000). Other reports are `noindex`. The text comes from a site anyone can point the tool at, so review what gets indexed before promoting the site. Set `MODEL_CHECK_SITE_URL` in production.

## Site badge

`/get-badge`: a site enters its domain and gets code that shows its latest check result.

| Route | What |
|---|---|
| `GET /embed/<domain>.js` | script for that domain; draws the badge in a shadow root (floating, or inside `<div id="modelsexam-badge">`) |
| `GET /badge/<domain>.svg` | the same badge as an image, for pages that render HTML but ignore scripts, and for Markdown |
| `GET /api/badge/<domain>` | JSON the script reads |
| `/sites/<domain>` | page the badge links to (latest result and report) |

The badge is built from the newest completed, scored check whose endpoint host is the domain or a subdomain. It expires after 30 days (`Check expired`). The JSON is served only to pages whose `Origin` is on that domain (or this service); an image requested with another site as `Referer` becomes a grey "Not issued for this site". A screenshot can always be copied, so the badge shows the domain and links to the public report.
