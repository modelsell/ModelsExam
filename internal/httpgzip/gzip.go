// Package httpgzip compresses text responses (HTML, JS, CSS, JSON, SVG, XML)
// for clients that accept gzip. Streams such as server-sent events pass
// through untouched so they keep flushing immediately.
package httpgzip

import (
	"compress/gzip"
	"mime"
	"net/http"
	"strings"
	"sync"
)

var pool = sync.Pool{New: func() any {
	w, _ := gzip.NewWriterLevel(nil, gzip.DefaultCompression)
	return w
}}

func compressible(contentType string) bool {
	mt, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	switch {
	case mt == "text/event-stream":
		return false
	case strings.HasPrefix(mt, "text/"):
		return true
	case mt == "application/json", mt == "application/javascript", mt == "application/xml", mt == "image/svg+xml":
		return true
	}
	return false
}

// Handler wraps next.
func Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead || r.Header.Get("Range") != "" ||
			!strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}
		gw := &writer{ResponseWriter: w}
		defer gw.close()
		next.ServeHTTP(gw, r)
	})
}

type writer struct {
	http.ResponseWriter
	gz          *gzip.Writer
	wroteHeader bool
}

func (w *writer) WriteHeader(code int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	h := w.Header()
	if h.Get("Content-Encoding") == "" && code != http.StatusNoContent && code != http.StatusNotModified &&
		code >= 200 && compressible(h.Get("Content-Type")) {
		h.Del("Content-Length")
		h.Set("Content-Encoding", "gzip")
		h.Add("Vary", "Accept-Encoding")
		w.gz = pool.Get().(*gzip.Writer)
		w.gz.Reset(w.ResponseWriter)
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *writer) Write(p []byte) (int, error) {
	if !w.wroteHeader {
		if w.Header().Get("Content-Type") == "" {
			w.Header().Set("Content-Type", http.DetectContentType(p))
		}
		w.WriteHeader(http.StatusOK)
	}
	if w.gz != nil {
		return w.gz.Write(p)
	}
	return w.ResponseWriter.Write(p)
}

func (w *writer) Flush() {
	if w.gz != nil {
		_ = w.gz.Flush()
	}
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *writer) close() {
	if w.gz != nil {
		_ = w.gz.Close()
		pool.Put(w.gz)
		w.gz = nil
	}
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (w *writer) Unwrap() http.ResponseWriter { return w.ResponseWriter }
