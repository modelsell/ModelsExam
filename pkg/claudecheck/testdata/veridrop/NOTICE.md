# Veridrop attribution

Source: https://github.com/canarybyte/veridrop
Pinned revision: `16feef72ad76d154c3ae6b4c0917319597fc1888` (2026-08-15).
License: GNU Affero General Public License v3.0; see LICENSE in this directory.
Original project and contributors: canarybyte/veridrop.

`test_document.pdf` is copied unchanged from
`src/relay_detector/protocols/anthropic/data/test_document.pdf`.
`baselines.json` is a curated derivative of the four `data/baselines/*_full.json`
reports. It retains published fixture expectations and selected numerical
observations, original file SHA-256, collection date, model and provenance.
API key fragments, generated text, signatures, original scores and verdicts
are omitted. The upstream describes these as official Anthropic API samples;
this project has not independently reproduced their collection.

`../../semantic_probes.go` adapts the PDF, structured_output and integrity probe
mechanisms into Go. Changes: no temperature parameter (model compatibility),
semantic JSON comparison including fenced JSON, no signature-length,
ID-prefix or character/token heuristics used as authentication or scoring.
Numeric references are historical observations, not billing truth or thresholds.
The three-run HTTP 418 token sequence is reference-only and is not compared
with this project's different PONG repeatability probe.

Integration update: tool schema validation is part of the regular tool probe. PDF checks now use a locally generated neutral document with a fresh identifier. Stream semantics and stop reasons are separate checks. Historical fixtures remain archived here; they are not loaded by current runs or shown in reports.
