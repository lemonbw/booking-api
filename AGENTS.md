# booking-api

Go 1.25 backend for room bookings. PostgreSQL 17, pgx/v5, vendored dependencies.

## Code style

No comments in Go source. Not doc comments, not inline, not teaching notes.
The only exception is a compiler directive (`//go:embed`). Tests must not
carry explanatory comments either, the assertion names carry the intent.
If a comment ever lands in a commit, say so before the next commit lands so
history can be rewritten before it grows tags.

Long-term intent lives in `README.md`, `CHANGELOG.md` and the contributor notes,
not above the code.

## Layout

- `cmd/app` - composition root, wiring only, no logic
- `internal/domain` - entities and domain errors, no imports outside stdlib
- `internal/usecase` - orchestration and authorisation, depends on ports
  - `booking_service.go` - service, CRUD paths, paging
  - `series.go` - recurring occurrences
  - `principal.go` - identity and ownership rules
  - `ownership.go` - room checks, overlap preflight, error interpretation
  - `retention.go` - background purge
  - `ports.go` - repository and platform ports
- `internal/adapter/httpapi` - HTTP transport, status codes, error mapping
  - `router.go` - routes, probes, json fallback for 404/405
  - `handler.go` - parse and delegate, no business rules
  - `body.go` - body limit and strict json decoding
  - `dto.go` - request and response shapes
  - `auth.go` / `auth_middleware.go` - credentials and the gate
  - `accesslog.go`, `requestid.go`, `trace.go`, `metrics_middleware.go`, `route_pattern.go`
- `internal/adapter/postgres` - SQL, migrations, SQLSTATE translation
- `internal/platform` - clock, idgen, logger, metrics, version
  - `metrics` - registry split into `registry`, `counter`, `histogram`, `gauge`, `text`

## Ground rules

- Verify all checks with `make fmt`, `make vet`, `make test` before every commit.
- Conventional Commits, one logical change per commit.
- Keep files focused: one topic per file, no file over ~300 lines.
- Every minor release is followed by at least one real bugfix patch.
- The domain layer must never import another project package.
- SQL is built from parameters only; identifiers never come from a request.
- Tests: table driven, assertion names carry the intent, no comments.
- Integration tests skip themselves when `DATABASE_URL` is absent and clean up
  their own rows.
