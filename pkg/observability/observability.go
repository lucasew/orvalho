package observability

import (
	"log"
)

// ReportError provides a centralized error-reporting function.
// All code paths that handle unexpected errors MUST funnel through this function.
func ReportError(err error, contextMsg string) {
	if err == nil {
		return
	}
	// Currently logs to standard logger. Could be extended to use Sentry or other backends.
	log.Printf("ERROR: %s: %v\n", contextMsg, err)
}
