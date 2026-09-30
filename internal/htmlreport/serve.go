package htmlreport

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// LoadTimeout is how long Serve waits for the browser to load the page. Tests override it.
var LoadTimeout = 5 * time.Minute

// shutdownGrace bounds how long Serve lets the finished response drain.
const shutdownGrace = 5 * time.Second

// pageCSP repeats the page's CSP meta tag as a header and adds directives only a header can carry.
const pageCSP = "default-src 'none'; style-src 'unsafe-inline'; img-src data:; frame-ancestors 'none'"

// Serve serves page once on 127.0.0.1 under an unguessable path and calls open with the URL.
// It returns nil after the first complete GET of that path, an error after LoadTimeout, or ctx.Err() when ctx ends first.
// A failing open only prints a warning with the URL to stderr.
func Serve(ctx context.Context, page []byte, open func(url string) error, stderr io.Writer) error {
	token, err := newToken()
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	url := "http://" + ln.Addr().String() + "/" + token

	h := &reportHandler{path: "/" + token, page: page, served: make(chan struct{})}
	srv := &http.Server{
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		ErrorLog:          log.New(io.Discard, "", 0),
	}
	go srv.Serve(ln)
	timer := time.NewTimer(LoadTimeout)
	defer timer.Stop()

	fmt.Fprintf(stderr, "Report: %s\n", url)
	if err := open(url); err != nil {
		fmt.Fprintf(stderr, "skill-atlas: couldn't open a browser: %v\nOpen this URL yourself: %s\n", err, url)
	}

	select {
	case <-h.served:
		sctx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
		defer cancel()
		if srv.Shutdown(sctx) != nil {
			srv.Close()
		}
		return nil
	case <-timer.C:
		srv.Close()
		return fmt.Errorf("the browser didn't load the report within %s", LoadTimeout)
	case <-ctx.Done():
		srv.Close()
		return ctx.Err()
	}
}

// newToken returns 128 random bits as hex.
func newToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// reportHandler answers GET on its path and 404 elsewhere; served closes after the first complete response.
type reportHandler struct {
	path   string
	page   []byte
	served chan struct{}
	once   sync.Once
}

func (h *reportHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if subtle.ConstantTimeCompare([]byte(r.URL.Path), []byte(h.path)) != 1 {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	hdr := w.Header()
	hdr.Set("Content-Type", "text/html; charset=utf-8")
	hdr.Set("Content-Length", strconv.Itoa(len(h.page)))
	hdr.Set("Content-Security-Policy", pageCSP)
	hdr.Set("Cache-Control", "no-store")
	hdr.Set("Referrer-Policy", "no-referrer")
	hdr.Set("X-Content-Type-Options", "nosniff")
	if _, err := w.Write(h.page); err != nil {
		return
	}
	if err := http.NewResponseController(w).Flush(); err != nil {
		return
	}
	h.once.Do(func() { close(h.served) })
}
