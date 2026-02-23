package observability

import (
	"fmt"
	"os"
)

// ReportError reports an unexpected error to the centralized error tracking system.
// It is intended for background processes or internal errors that cannot be returned to the caller.
// Currently, it logs to stderr, but should be replaced with a Sentry/Rollbar client in production.
func ReportError(err error, context ...interface{}) {
	if err == nil {
		return
	}
	// In a real application, this would send to Sentry/Rollbar etc.
	// We use stderr to separate errors from standard output.
	fmt.Fprintf(os.Stderr, "ERROR: %v context=%v\n", err, context)
}
