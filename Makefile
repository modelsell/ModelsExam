.PHONY: web static build run test dev-api
web:
	cd web && bun install && bun run build
# Static HTML for the pages that need no database (set MODEL_CHECK_SITE_URL).
static: web
	go run ./cmd/seostatic
build: web
	go build -o modelsexam ./cmd/model-check
run: build
	./modelsexam
test:
	go test ./...
	cd web && bun run typecheck && bun run test
# API only on :8080 (run `cd web && bun run dev` alongside: serves on :5178, proxies /api).
dev-api:
	# A fixed, public development master key: never use it in production.
	MODEL_CHECK_ALLOW_PRIVATE=true AUTH_REQUIRE_HTTPS=false MODEL_CHECK_SECRET_KEY=ZGV2LW9ubHktbW9kZWxzZXhhbS1tYXN0ZXIta2V5ISE= go run ./cmd/model-check
