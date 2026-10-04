# SEO plan

Goal: a visitor who searches a site's name, a domain, a model name, or words
like "模型评测" / "API 真伪检测" / "relay check" should land on a ModelsExam page
that already contains the answer in its HTML.

## How pages reach a search engine

The app is a single-page app, so the Go server writes the real content into the
HTML of every public route (`internal/seo`, `internal/server/seopages.go`):
title, description, canonical, Open Graph, JSON-LD and a plain-HTML body. React
replaces the body when it starts. Anything a crawler needs must be in that body.

## Page types

| URL | Targets | Indexable when |
| --- | --- | --- |
| `/` | brand, 模型评测, API 真伪检测 | always; lists the 20 latest records, top models, recent sites |
| `/records`, `/records?page=N` | "check records" | always; 30 per page, canonical per page, prev/next links |
| `/reports/<id>` | "<site> <model> 模型评测" | run completed and the site name was read |
| `/sites/<domain>` | the site's name and domain | at least one indexable record |
| `/models/<name>` | "<model> 模型评测", "<model> 真伪" | at least one indexable record; name must be path-safe |
| `/baselines`, `/method`, `/get-badge` | explainer queries | always |

Reports, sites and models are linked to each other and from the home page and
the records table, so every page is reachable by links, not only by sitemap.

## Content rules

- Titles and descriptions carry Chinese and English keywords.
- Bodies state facts of the run (site, endpoint, source, protocol, model,
  result, date, requests, passed/failed, duration) and say plainly that a
  result is evidence about one run, not a ranking or endorsement.
- Site name and description come from the tested site's own home page, fetched
  at check time. They are untrusted text and are escaped everywhere.
- Unfinished runs and runs without a site name are `noindex`.
- Unknown report ids, unknown models and out-of-range record pages return 404.
- The app never overrides the robots tag of report, site or model pages.

## Done (phase 1)

Rich report body, paginated records list, indexable site and model pages,
JSON-LD (BreadcrumbList, ItemList), sitemap with `lastmod`, home-page links.

## Next

1. Chinese URLs: serve `/zh/...` with Chinese body text and `hreflang`
   alternates; today the server body is English plus Chinese keywords.
2. Put the per-check results (name, pass/fail) of a report into its HTML; needs
   a stable summary of `ReportJSON` for the three report types.
3. A real Open Graph image (PNG) per page type.
4. Split the sitemap into an index once it passes 50,000 URLs.
5. Set `MODEL_CHECK_SITE_URL`, then submit the sitemap to Google Search Console
   and Baidu Webmaster, and verify with "URL inspection".
6. Guides that match how people search ("how to tell a fake Claude relay").
7. Watch Core Web Vitals; the JS bundle is the main cost of the app shell.

## Phase 2: content structure (reference: hvoyai.com, veridrop.org)

Chinese-first content with keyword-led titles; every page is server-rendered
plain HTML for crawlers and replaced by React on mount.

| Path | Purpose | Data |
| --- | --- | --- |
| `/` | hero, live stats, top model boards, promises, badge, baselines, FAQ (FAQPage JSON-LD), latest records | `internal/server/boards.go` snapshot |
| `/models`, `/models/<m>` | per-model board (newest scored result per site, best first, expired after 30 days) + all records | `internal/board` |
| `/sites/<host>` | per-site history (no directory index) | store |
| `/reports/<id>` | one report, indexable when completed and named | store |

Honesty rules carried over from the reference sites: boards show dated results,
not endorsements; stats are real database counts and are hidden until the first
check exists; no "blacklist" wording.

APIs (all GET, cached 60 s): `/api/model_check/stats`, `/boards`, `/boards/:model`,
`/boards`, `/boards/:model`. The site directory and guides were removed.

To do: translate the new strings for zh-TW/ja/fr/ru/vi; a `/zh/` URL prefix with
hreflang; fill baseline report ids; add more guides as real questions arrive.

## Phase 3: audit and hardening (2026-10-04)

Checked every page type with `internal/seo/lint_test.go` (runs in `go test`).

Found and fixed:

- `<html lang="en">` on Chinese pages; client code set `lang="zhCN"` (invalid). Now `zh-CN`.
- The app overwrote the server's head on first load: `/records?page=2` lost its canonical, `twitter:card` was reset. The app now leaves a server-rendered head alone.
- Default UI language followed the browser (English for Googlebot) while the HTML was Chinese. The app now starts in Chinese unless the visitor chose another language; other locales load on demand.
- No share image, `twitter:card=summary`, no hreflang, no theme-color, no Organization data. Added `og-image.png` (1200x630), `logo.png`, `summary_large_image`, `og:locale`, hreflang `zh-CN` and `x-default`, Organization + WebSite JSON-LD.
- Paginated `/records?page=N` shared one description. Now unique.
- Theme script sat outside `<head>` in the HTML template. Moved inside.
- No compression. `internal/httpgzip` compresses text responses (not event streams).
- Heavy first load: pages other than home, and the report drawer, are lazy chunks; locales other than Chinese are separate chunks.
- Static build of the database-independent pages: `make static`, see `static-deploy.md`.

Lint now guards: one h1, no heading jumps, title and description length and
uniqueness, canonical and robots consistency, og/twitter tags, valid JSON-LD,
every internal link resolves, sitemap equals the set of indexable pages.

Open: submit sitemap (Google Search Console, Baidu); site verification meta
tags are not configurable yet; measure Core Web Vitals after `make web`.
