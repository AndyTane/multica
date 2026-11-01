// Package secondmachine holds the end-to-end verification for the second-machine
// Windows package (CLAWSTA-104): install the single-file installer into a clean
// user profile, run one connect command against a fake HTTPS deployment whose
// certificate is issued by a private CA, and prove the daemon comes online and
// registers a runtime — with no real model calls and no real deployment.
//
// The test itself is behind the secondmachine_e2e build tag so the normal test
// run never spawns a daemon or opens ports:
//
//	go test -tags secondmachine_e2e ./e2e/secondmachine -run TestSecondMachine -v
package secondmachine
