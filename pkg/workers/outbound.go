package workers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/dop251/goja"
)

// MaxOutboundBody is the maximum response body retained for guest fetch (2 MiB).
const MaxOutboundBody = 2 << 20

// DefaultFetchTimeout bounds a single outbound fetch when no shorter deadline
// is present on the active context.
const DefaultFetchTimeout = 15 * time.Second

// HTTPFetch returns a [FetchFunc] backed by net/http with an egress allowlist.
// Empty egress denies all destinations. client may be nil (default client with
// redirect checks against egress). timeout zero uses DefaultFetchTimeout.
func HTTPFetch(egress EgressList, client *http.Client, timeout time.Duration) FetchFunc {
	if timeout <= 0 {
		timeout = DefaultFetchTimeout
	}
	if client == nil {
		client = &http.Client{
			Timeout: timeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 10 {
					return ErrTooManyRedirects
				}
				if err := egress.CheckURL(req.URL.String()); err != nil {
					return err
				}
				return nil
			},
		}
	}
	return func(ctx context.Context, req *http.Request) (*http.Response, error) {
		if err := egress.CheckURL(req.URL.String()); err != nil {
			return nil, err
		}
		// Caller owns deadlines on ctx (jsFetch applies FetchTimeout).
		// Do not cancel here: response body is read after this returns.
		req = req.WithContext(ctx)
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		// Re-check final URL after redirects.
		if resp.Request != nil && resp.Request.URL != nil {
			if err := egress.CheckURL(resp.Request.URL.String()); err != nil {
				resp.Body.Close()
				return nil, err
			}
		}
		return resp, nil
	}
}

func (iso *Isolate) installOutboundFetch() {
	if iso.opts.Fetch == nil {
		return
	}
	iso.vm.Set("fetch", iso.jsFetch)
}

// jsFetch implements the guest global fetch(input, init?).
// The HTTP round-trip runs synchronously under the isolate lock (host-driven
// model). The return value is always a Promise (fulfilled or rejected).
func (iso *Isolate) jsFetch(call goja.FunctionCall) goja.Value {
	p, resolve, reject := iso.vm.NewPromise()
	promise := iso.vm.ToValue(p)
	start := time.Now()

	reqURL, method, headers, body, err := iso.parseFetchArgs(call)
	if err != nil {
		logGuestFetch(method, reqURL, 0, 0, start, err)
		reject(iso.vm.NewTypeError(err.Error()))
		return promise
	}

	ctx := iso.activeCtx
	if ctx == nil {
		// Outbound fetch only runs while a host Fetch/Tick holds the isolate.
		logGuestFetch(method, reqURL, 0, 0, start, ErrFetchRequiresURL)
		reject(iso.vm.NewTypeError("fetch outside host request context"))
		return promise
	}
	timeout := iso.opts.FetchTimeout
	if timeout <= 0 {
		timeout = DefaultFetchTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var bodyReader io.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}
	httpReq, err := http.NewRequestWithContext(ctx, method, reqURL, bodyReader)
	if err != nil {
		logGuestFetch(method, reqURL, 0, 0, start, err)
		reject(iso.vm.NewTypeError(err.Error()))
		return promise
	}
	for k, v := range headers {
		httpReq.Header.Set(k, v)
	}
	if body != "" && httpReq.Header.Get("Content-Type") == "" && method != http.MethodGet && method != http.MethodHead {
		httpReq.Header.Set("Content-Type", "text/plain;charset=UTF-8")
	}

	resp, err := iso.opts.Fetch(ctx, httpReq)
	if err != nil {
		logGuestFetch(method, reqURL, 0, 0, start, fmt.Errorf("fetch failed: %w", err))
		reject(iso.vm.NewTypeError("fetch failed: " + err.Error()))
		return promise
	}
	defer resp.Body.Close()

	finalURL := reqURL
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL.String()
	}

	limited := io.LimitReader(resp.Body, MaxOutboundBody+1)
	raw, err := io.ReadAll(limited)
	if err != nil {
		logGuestFetch(method, finalURL, resp.StatusCode, 0, start, fmt.Errorf("read body: %w", err))
		reject(iso.vm.NewTypeError("fetch read body: " + err.Error()))
		return promise
	}
	if len(raw) > MaxOutboundBody {
		logGuestFetch(method, finalURL, resp.StatusCode, len(raw), start, ErrResponseBodyTooLarge)
		reject(iso.vm.NewTypeError("fetch response body too large"))
		return promise
	}

	logURL := reqURL
	if finalURL != "" && finalURL != reqURL {
		logURL = reqURL + " => " + finalURL
	}
	logGuestFetch(method, logURL, resp.StatusCode, len(raw), start, nil)

	resObj := iso.newResponseFromHTTPLocked(resp.StatusCode, resp.Status, resp.Header, string(raw))
	resolve(resObj)
	return promise
}

// logGuestFetch writes one line about a guest-originated outbound fetch.
func logGuestFetch(method, url string, status, bodyBytes int, start time.Time, err error) {
	if method == "" {
		method = "?"
	}
	if url == "" {
		url = "<invalid>"
	}
	dur := time.Since(start).Round(time.Millisecond)
	if err != nil {
		fmt.Fprintf(os.Stderr, "workers fetch: %s %s -> error: %v (%s)\n", method, url, err, dur)
		return
	}
	fmt.Fprintf(os.Stderr, "workers fetch: %s %s -> %d %dB %s\n", method, url, status, bodyBytes, dur)
}

func (iso *Isolate) parseFetchArgs(call goja.FunctionCall) (url, method string, headers map[string]string, body string, err error) {
	if len(call.Arguments) < 1 {
		return "", "", nil, "", ErrFetchRequiresURL
	}
	method = http.MethodGet
	headers = make(map[string]string)

	arg0 := call.Argument(0)
	if obj, ok := arg0.(*goja.Object); ok {
		if r, ok := iso.requestReg[obj]; ok {
			url = r.url
			method = r.method
			if method == "" {
				method = http.MethodGet
			}
			for k, v := range r.headers.m {
				headers[k] = v
			}
			body = r.body
		} else {
			url = arg0.String()
		}
	} else {
		url = arg0.String()
	}

	if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) && !goja.IsNull(call.Argument(1)) {
		init := call.Argument(1).ToObject(iso.vm)
		if v := init.Get("method"); v != nil && !goja.IsUndefined(v) {
			method = strings.ToUpper(v.String())
		}
		if v := init.Get("headers"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
			h := newHeaderBag()
			if err := iso.fillHeaders(h, v); err != nil {
				return "", "", nil, "", err
			}
			headers = h.asMap()
		}
		if v := init.Get("body"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
			body = v.String()
		}
	}

	if url == "" {
		return "", "", nil, "", ErrFetchURLEmpty
	}
	if method == "" {
		method = http.MethodGet
	}
	return url, method, headers, body, nil
}

func (iso *Isolate) newResponseFromHTTPLocked(status int, statusLine string, hdr http.Header, body string) *goja.Object {
	statusText := ""
	if _, text, ok := strings.Cut(statusLine, " "); ok {
		statusText = text
	}
	h := newHeaderBag()
	for k, vals := range hdr {
		if len(vals) == 0 {
			continue
		}
		h.set(k, strings.Join(vals, ", "))
	}
	r := &responseBag{
		status:     status,
		statusText: statusText,
		headers:    h,
		body:       body,
	}
	ctor, ok := goja.AssertConstructor(iso.vm.Get("Response"))
	if !ok {
		panic("Response constructor missing")
	}
	obj, err := ctor(nil)
	if err != nil {
		panic(err)
	}
	iso.responseReg[obj] = r
	return obj
}
