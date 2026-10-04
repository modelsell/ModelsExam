# Contributing to ModelsExam

1. Open an issue before large changes (new check suites, scoring changes, new providers).
2. With Go, Bun and Node.js installed, build and test: `make build && make test && go vet ./...`
   (frontend build, Go build/tests, TypeScript typecheck and frontend unit tests).
3. Keep the three check kinds separate: assertions (count toward the score), observations and
   provenance (never change a badge). Requests are never retried.
4. New user-visible strings need keys in `web/src/i18n/locales/en.json` and `zh.json`.
5. Never log or persist API keys; reports must redact them.
6. Contributions are accepted under AGPL-3.0, the project license. Keep existing file headers.

Be respectful and constructive. Conduct follows the Contributor Covenant 2.1
(https://www.contributor-covenant.org/version/2/1/code_of_conduct/).
