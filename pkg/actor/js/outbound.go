package js

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/dop251/goja"
)

// MaxOutboundBody is the maximum response body retained for guest fetch (2 MiB).
const MaxOutboundBody = 2 << 20

// DefaultFetchTimeout bounds a single outbound fetch when no shorter deadline
// is present on the active context.
const DefaultFetchTimeout = 15 * time.Second

func (iso *Isolate) installOutboundFetch() {
	iso.vm.Set("fetch", iso.jsFetch)
}

// jsFetch implements the guest global fetch(input, init?).
// The HTTP round-trip runs synchronously under the isolate lock (host-driven
// model). The return value is always a Promise (fulfilled or rejected).
func (iso *Isolate) jsFetch(call goja.FunctionCall) goja.Value {
	p, resolve, reject := iso.vm.NewPromise()
	promise := iso.vm.ToValue(p)

	reqURL, method, headers, body, err := iso.parseFetchArgs(call)
	if err != nil {
		_ = reject(iso.vm.NewTypeError(err.Error()))
		return promise
	}
	if err := iso.opts.Egress.CheckURL(reqURL); err != nil {
		_ = reject(iso.vm.NewTypeError(err.Error()))
		return promise
	}

	ctx := iso.activeCtx
	if ctx == nil {
		ctx = context.Background()
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
		_ = reject(iso.vm.NewTypeError(err.Error()))
		return promise
	}
	for k, v := range headers {
		httpReq.Header.Set(k, v)
	}
	if body != "" && httpReq.Header.Get("Content-Type") == "" && method != http.MethodGet && method != http.MethodHead {
		httpReq.Header.Set("Content-Type", "text/plain;charset=UTF-8")
	}

	client := iso.httpClient()
	resp, err := client.Do(httpReq)
	if err != nil {
		_ = reject(iso.vm.NewTypeError("fetch failed: " + err.Error()))
		return promise
	}
	defer resp.Body.Close()

	// Re-check final URL after redirects.
	if err := iso.opts.Egress.CheckURL(resp.Request.URL.String()); err != nil {
		_ = reject(iso.vm.NewTypeError(err.Error()))
		return promise
	}

	limited := io.LimitReader(resp.Body, MaxOutboundBody+1)
	raw, err := io.ReadAll(limited)
	if err != nil {
		_ = reject(iso.vm.NewTypeError("fetch read body: " + err.Error()))
		return promise
	}
	if len(raw) > MaxOutboundBody {
		_ = reject(iso.vm.NewTypeError("fetch response body too large"))
		return promise
	}

	resObj := iso.newResponseFromHTTPLocked(resp.StatusCode, resp.Status, resp.Header, string(raw))
	_ = resolve(resObj)
	return promise
}

func (iso *Isolate) httpClient() *http.Client {
	if iso.opts.HTTPClient != nil {
		return iso.opts.HTTPClient
	}
	if iso.defaultClient == nil {
		iso.defaultClient = &http.Client{
			Timeout: DefaultFetchTimeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 10 {
					return fmt.Errorf("stopped after 10 redirects")
				}
				if err := iso.opts.Egress.CheckURL(req.URL.String()); err != nil {
					return err
				}
				return nil
			},
		}
	}
	return iso.defaultClient
}

func (iso *Isolate) parseFetchArgs(call goja.FunctionCall) (url, method string, headers map[string]string, body string, err error) {
	if len(call.Arguments) < 1 {
		return "", "", nil, "", fmt.Errorf("fetch requires a URL or Request")
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
		return "", "", nil, "", fmt.Errorf("fetch URL is empty")
	}
	if method == "" {
		method = http.MethodGet
	}
	return url, method, headers, body, nil
}

func (iso *Isolate) newResponseFromHTTPLocked(status int, statusLine string, hdr http.Header, body string) *goja.Object {
	statusText := ""
	if parts := strings.SplitN(statusLine, " ", 2); len(parts) == 2 {
		statusText = parts[1]
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
