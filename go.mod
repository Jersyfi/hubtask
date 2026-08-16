module github.com/Jersyfi/hubtask

go 1.25.0

// The toolchain is pinned to a patched release, not just to a major version: gate-security runs
// govulncheck, and an unpatched standard library is a finding there. Whoever builds with an older
// Go gets this toolchain fetched automatically.
toolchain go1.25.13

// Dependencies are added step by step from milestone 0.1.0 onwards.
// The core (core/domain, core/port, core/shared) stays permanently free of
// third-party dependencies - enforced by test/architecture.
