module github.com/Jersyfi/hubtask

go 1.25.0

// The toolchain is pinned to a patched release, not just to a major version: gate-security runs
// govulncheck, and an unpatched standard library is a finding there. Whoever builds with an older
// Go gets this toolchain fetched automatically.
toolchain go1.25.13

// Dependencies are added step by step from milestone 0.1.0 onwards.
// The core (core/domain, core/port, core/shared) stays permanently free of
// third-party dependencies - enforced by test/architecture.

require github.com/jackc/pgx/v5 v5.10.0

require (
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	golang.org/x/sync v0.17.0 // indirect
	golang.org/x/text v0.29.0 // indirect
)
