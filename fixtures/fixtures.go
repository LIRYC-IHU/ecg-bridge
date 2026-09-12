// Package fixtures gates the tests that can only run against real recordings.
//
// Decode fidelity is verified by comparing decoded samples against a vendor's
// own reference export. Those inputs are patient recordings, so they are not in
// this repository and never will be: .gitignore refuses them by extension. The
// consequence is that on a public CI runner every one of those tests skips, and
// `go test ./...` reports success having compared no samples at all.
//
// That is a reasonable trade only if it is visible. Two things make it so:
//
//   - Require() turns a missing fixture into a failure when
//     ECGBRIDGE_REQUIRE_FIXTURES is set, so a machine that does hold the
//     recordings — a release build, a maintainer's checkout — cannot skip them
//     by accident.
//   - The CI workflow lists every skipped test by name in its summary, so a
//     green run says which formats were actually exercised.
//
// Tests written against synthesised inputs (see the round-trip tests in
// nk-to-fda and fukuda-to-fda) do not use this package: they run everywhere and
// are what CI actually proves.
package fixtures

import (
	"os"
	"testing"
)

// EnvRequire, when set to a non-empty value, converts every fixture-gated skip
// into a test failure.
const EnvRequire = "ECGBRIDGE_REQUIRE_FIXTURES"

// Required reports whether fixture-gated tests must run.
func Required() bool { return os.Getenv(EnvRequire) != "" }

// Require skips the test because an input it needs is absent — unless
// EnvRequire is set, in which case the absence is the failure.
//
// what names the missing input; why says what it would have verified, so the
// skip line in a CI log states which check did not run.
func Require(t *testing.T, what, why string) {
	t.Helper()
	if Required() {
		t.Fatalf("%s is set but %s is missing; %s was not verified", EnvRequire, what, why)
	}
	t.Skipf("%s not present — %s not verified (set %s to make this a failure)", what, why, EnvRequire)
}
