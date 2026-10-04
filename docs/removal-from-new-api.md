# Historical checklist: removing model check from new-api

This checklist was written during extraction from new-api. It records the files and routes in
that source checkout, rather than the current state of any new-api installation. Verify the
standalone project first and review the relevant new-api revision before applying it. Keep the
removal on its own branch/commit so unrelated changes are preserved.

## 0. Verify the standalone project first

```bash
cd model-check
go mod tidy && go vet ./... && go test ./...
cd web && bun install && bun run typecheck && bun run test && bun run build && cd ..
make build && ./modelsexam          # open http://localhost:8080, run a Claude and an OpenAI check
```

## 1. Backend (Go) — delete

- `controller/channel_claude_check.go`, `channel_claude_check_transport.go`, `claude_check_keepalive.go`
- `controller/model_check_baseline.go`, `model_check_history.go`, `model_check_openai.go`, `model_check_openai_history.go`
- matching `*_test.go` for each of the above
- `model/model_check_run.go`, `model/model_check_baseline.go`, `model/model_check_run_test.go`
- `service/model_check_client.go` (+test). `service/protected_fetch_client.go` is shared with other
  fetchers — **keep**.
- `router/model-check-router.go`, `router/model_check_routes_test.go`
- `pkg/claudecheck/`, `pkg/openaicheck/`

## 2. Backend — edit

| File | Change |
|---|---|
| `router/channel-router.go:20` | remove `registerModelCheckRoutes(apiRouter)` |
| `router/channel-router.go:53` | remove the `/:id/claude_check` route entry |
| `router/channel_router_test.go:16`, `router/channel_routes_test.go:61` | drop the `claude_check` assertions |
| `middleware/workspace_account_policy.go:62-67` | remove the six `/api/model_check…` entries |
| `model/main.go:339-340, 459-460` | remove `ModelCheckRun` / `ModelCheckBaseline` from AutoMigrate and the migration list (the tables stay in the DB; drop them manually if you want) |
| `model/utils.go`, `service/system_task.go`, `main.go` | check `git diff` — they were touched for unrelated work; remove only model-check references if any |
| `router/seo.go` (230, 259, 549, 1136, 1542), `router/public_ssr.go:234`, `router/llms.go:108`, `router/web-router.go:88` + their tests (`llms_test.go`, `public_ssr_test.go`, `web_router_test.go`) | remove the `/model-check` page from SEO shell links, sitemap, SSR allow-list, llms.txt and the `view=capabilities` redirect |
| `go.mod` | `go mod tidy` (the AWS Bedrock SDK may still be used by the relay; leave it if so) |

## 3. Frontend (`web/default`) — delete

- `src/features/model-check/` (whole directory), `src/routes/model-check/`
- regenerate `src/routeTree.gen.ts` (it references the route)

## 4. Frontend — edit

`features/home/hooks/use-landing-footer-links.ts`, `features/system-settings/maintenance/{header-navigation-section.tsx,config.ts}`,
`features/channels/components/data-table-row-actions.tsx` (the "Claude check" row action),
`components/layout/components/use-site-navigation.ts`, `hooks/use-sidebar-data.test.ts`,
`lib/{seo.ts,seo.test.ts,nav-modules.ts,nav-modules-capabilities.test.ts,workspace-account.ts,public-bootstrap.ts}`,
and the model-check strings in `src/i18n/locales/*.json` / `static-keys.ts`.

Note: the `/model-capabilities` page and the `modelCapabilities` nav module are linked to the model-check
landing (`CheckExplore`); decide whether they stay.

## 5. Docs

`docs/model-check-*.md`, `docs/openai-model-check.md`, `docs/claude-*check*.md` — copies are in
`model-check/docs/`; the originals can go.

## 6. Re-verify new-api

`go build ./... && go test ./router/ ./controller/ ./model/ ./middleware/` and
`cd web/default && bun run typecheck && bun run build`, then
`grep -rniE "model[-_]check|claudecheck|openaicheck" --exclude-dir=node_modules --exclude-dir=dist --exclude-dir=logs --exclude-dir=outputs .`
should return nothing outside `docs/`.
