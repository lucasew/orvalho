package workers

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// MaxRequestBody is the maximum inbound body size for [Handler] (1 MiB).
const MaxRequestBody = 1 << 20

// Handler returns an http.Handler that maps each request to default.fetch.
// Requests are serialized through the isolate lock (single-threaded actor).
// Each request is logged to stderr (method, path, status, duration, body size).
func Handler(iso *Isolate) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		defer r.Body.Close()
		body, err := io.ReadAll(io.LimitReader(r.Body, MaxRequestBody+1))
		if err != nil {
			http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
			fmt.Fprintf(os.Stderr, "workers: %s %s -> 400 (%v)\n", r.Method, r.URL.RequestURI(), err)
			return
		}
		if len(body) > MaxRequestBody {
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			fmt.Fprintf(os.Stderr, "workers: %s %s -> 413 body too large\n", r.Method, r.URL.RequestURI())
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
			fmt.Fprintf(os.Stderr, "workers: %s %s -> 500 error: %v (%s)\n",
				r.Method, r.URL.RequestURI(), err, time.Since(start).Round(time.Millisecond))
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
		n, _ := w.Write([]byte(res.Body))
		fmt.Fprintf(os.Stderr, "workers: %s %s -> %d %dB %s\n",
			r.Method, r.URL.RequestURI(), status, n, time.Since(start).Round(time.Millisecond))
	})
}
