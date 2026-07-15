package js

import (
	"io"
	"net/http"
	"strings"
)

// MaxRequestBody is the maximum inbound body size for [Handler] (1 MiB).
const MaxRequestBody = 1 << 20

// Handler returns an http.Handler that maps each request to default.fetch.
// Requests are serialized through the isolate lock (single-threaded actor).
func Handler(iso *Isolate) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		body, err := io.ReadAll(io.LimitReader(r.Body, MaxRequestBody+1))
		if err != nil {
			http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
			return
		}
		if len(body) > MaxRequestBody {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}

		headers := make(map[string]string, len(r.Header))
		for k, vals := range r.Header {
			if len(vals) == 0 {
				continue
			}
			headers[k] = strings.Join(vals, ", ")
		}

		url := r.URL.String()
		if r.URL.Scheme == "" {
			// Relative URL from the server; build an absolute form for Request.url.
			scheme := "http"
			if r.TLS != nil {
				scheme = "https"
			}
			host := r.Host
			if host == "" {
				host = "localhost"
			}
			path := r.URL.RequestURI()
			url = scheme + "://" + host + path
		}

		res, err := iso.Fetch(r.Context(), HTTPRequest{
			Method:  r.Method,
			URL:     url,
			Headers: headers,
			Body:    string(body),
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		for k, v := range res.Headers {
			w.Header().Set(k, v)
		}
		status := res.Status
		if status == 0 {
			status = http.StatusOK
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(res.Body))
	})
}
