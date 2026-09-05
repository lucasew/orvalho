package workers

import "errors"

// Sentinel errors for package workers. Prefer wrapping with %w so callers can
// use errors.Is instead of matching error strings.
var (
	ErrMissingDefaultExport = errors.New("workers: missing global default export (expected default.fetch)")
	ErrDefaultFetchNotFunc  = errors.New("workers: default.fetch is not a function")
	ErrFetchRejected        = errors.New("workers: fetch rejected")
	ErrFetchTimeout         = errors.New("workers: fetch promise timed out")
	ErrEmptyEnvKey          = errors.New("workers: empty env string key")
	ErrEmptyBindingName     = errors.New("workers: empty binding name")
	ErrEnvNameClash         = errors.New("workers: env name clash")
	ErrAssetsMissingFS      = errors.New("assets: missing FS")
	ErrRequestCtorMissing   = errors.New("Request constructor not installed")
	ErrResponseNull         = errors.New("response is null or undefined")
	ErrNotAResponse         = errors.New("value is not a Response")
	ErrFetchRequiresURL     = errors.New("fetch requires a URL or Request")
	ErrFetchURLEmpty        = errors.New("fetch URL is empty")
	ErrTooManyRedirects     = errors.New("stopped after 10 redirects")
	ErrResponseBodyTooLarge = errors.New("response body too large")
	ErrEgressDenied         = errors.New("egress denied")
	ErrEgressMissingHost    = errors.New("egress denied: missing host")
	ErrBindNilIsolate       = errors.New("workers: bind needs isolate runtime")
	ErrBindNilObject        = errors.New("workers: bind needs RuntimeObject")
	ErrModuleNotFound       = errors.New("workers: module not found")
	ErrModuleSpecifier      = errors.New("workers: invalid module specifier")
)
