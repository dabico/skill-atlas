package htmlreport

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

var reportURL = regexp.MustCompile(`^http://127\.0\.0\.1:\d+/[0-9a-f]{32}$`)

// setTimeout overrides LoadTimeout for one test. Tests using it must not run in parallel.
func setTimeout(t *testing.T, d time.Duration) {
	t.Helper()
	old := LoadTimeout
	LoadTimeout = d
	t.Cleanup(func() { LoadTimeout = old })
}

// serve runs Serve and calls act(url) on its own goroutine in place of a browser. It waits for act before returning.
func serve(ctx context.Context, page []byte, act func(url string)) (stderr string, err error) {
	var wg sync.WaitGroup
	var buf bytes.Buffer
	open := func(url string) error {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if act != nil {
				act(url)
			}
		}()
		return nil
	}
	err = Serve(ctx, page, open, &buf)
	wg.Wait()
	return buf.String(), err
}

func get(url string) (*http.Response, []byte, error) {
	c := &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{DisableKeepAlives: true}}
	resp, err := c.Get(url)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	return resp, body, err
}

func do(method, url string) (*http.Response, error) {
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, err
	}
	c := &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{DisableKeepAlives: true}}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	resp.Body.Close()
	return resp, nil
}

// assertGone fails when url still accepts connections.
func assertGone(t *testing.T, url string) {
	t.Helper()
	if resp, _, err := get(url); err == nil {
		t.Errorf("server still answers after Serve returned: %s", resp.Status)
	}
}

func TestServeFirstGet(t *testing.T) {
	page := []byte("<!doctype html><title>t</title><p>report</p>")
	var url string
	stderr, err := serve(context.Background(), page, func(u string) {
		url = u
		resp, body, err := get(u)
		if err != nil {
			t.Errorf("GET: %v", err)
			return
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("status = %s, want 200", resp.Status)
		}
		if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
			t.Errorf("Content-Type = %q, want text/html", ct)
		}
		if !bytes.Equal(body, page) {
			t.Errorf("body = %q, want the page", body)
		}
		for k, want := range map[string]string{
			"Cache-Control":          "no-store",
			"Referrer-Policy":        "no-referrer",
			"X-Content-Type-Options": "nosniff",
		} {
			if got := resp.Header.Get(k); got != want {
				t.Errorf("%s = %q, want %q", k, got, want)
			}
		}
		if csp := resp.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'none'") {
			t.Errorf("Content-Security-Policy = %q", csp)
		}
	})
	if err != nil {
		t.Fatalf("Serve: %v", err)
	}
	if !reportURL.MatchString(url) {
		t.Errorf("url = %q, want http://127.0.0.1:<port>/<32 hex digits>", url)
	}
	if want := "Report: " + url + "\n"; stderr != want {
		t.Errorf("stderr = %q, want %q", stderr, want)
	}
	assertGone(t, url)
}

func TestServeOtherPathsAre404(t *testing.T) {
	page := []byte("<p>secret report</p>")
	stderr, err := serve(context.Background(), page, func(u string) {
		token := u[strings.LastIndex(u, "/")+1:]
		base := u[:strings.LastIndex(u, "/")]
		for _, p := range []string{
			"/", "/favicon.ico", "/robots.txt", "/index.html", "/" + token[:31], "/" + token + "x",
			"/" + token + "/", "//" + token, "/x/" + token, "/" + token + "/../" + token,
		} {
			resp, body, err := get(base + p)
			if err != nil {
				t.Errorf("GET %s: %v", p, err)
				continue
			}
			if resp.StatusCode != http.StatusNotFound {
				t.Errorf("GET %s = %s, want 404", p, resp.Status)
			}
			if bytes.Contains(body, page) {
				t.Errorf("GET %s leaked the page", p)
			}
		}
		// Wrong methods on the right path don't consume the one load either.
		for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
			resp, err := do(m, u)
			if err != nil {
				t.Errorf("%s: %v", m, err)
				continue
			}
			if resp.StatusCode != http.StatusMethodNotAllowed {
				t.Errorf("%s = %s, want 405", m, resp.Status)
			}
		}
		// The server is still up and serves the real request.
		if resp, body, err := get(u); err != nil || resp.StatusCode != http.StatusOK || !bytes.Equal(body, page) {
			t.Errorf("GET after 404s: resp %v, err %v", resp, err)
		}
	})
	if err != nil {
		t.Fatalf("Serve: %v\n%s", err, stderr)
	}
}

func TestServeTokensDiffer(t *testing.T) {
	var urls [2]string
	for i := range urls {
		_, err := serve(context.Background(), []byte("x"), func(u string) {
			urls[i] = u
			get(u)
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if urls[0] == urls[1] {
		t.Errorf("two runs used the same url %s", urls[0])
	}
}

func TestServeTimeout(t *testing.T) {
	setTimeout(t, 50*time.Millisecond)
	var url string
	stderr, err := serve(context.Background(), []byte("x"), func(u string) { url = u })
	if err == nil {
		t.Fatal("Serve returned nil, want a timeout error")
	}
	if want := "the browser didn't load the report within 50ms"; err.Error() != want {
		t.Errorf("err = %q, want %q", err, want)
	}
	if !strings.Contains(stderr, "Report: "+url) {
		t.Errorf("stderr = %q lacks the report url", stderr)
	}
	assertGone(t, url)
}

func TestServeDefaultTimeoutMessage(t *testing.T) {
	if LoadTimeout != 5*time.Minute {
		t.Fatalf("default LoadTimeout = %s, want 5m0s", LoadTimeout)
	}
	if got := fmt.Sprintf("the browser didn't load the report within %s", LoadTimeout); got != "the browser didn't load the report within 5m0s" {
		t.Errorf("message = %q", got)
	}
}

func TestServeCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var url string
	_, err := serve(ctx, []byte("x"), func(u string) {
		url = u
		time.Sleep(20 * time.Millisecond)
		cancel()
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	assertGone(t, url)
}

func TestServeOpenFailureWarnsAndKeepsWaiting(t *testing.T) {
	page := []byte("<p>report</p>")
	var wg sync.WaitGroup
	var url string
	var buf bytes.Buffer
	open := func(u string) error {
		wg.Add(1)
		go func() {
			defer wg.Done()
			url = u
			time.Sleep(50 * time.Millisecond) // the user opens the printed URL by hand
			if _, body, err := get(u); err != nil || !bytes.Equal(body, page) {
				t.Errorf("GET after failed open: %q, %v", body, err)
			}
		}()
		return errors.New("no browser here")
	}
	if err := Serve(context.Background(), page, open, &buf); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	wg.Wait()
	out := buf.String()
	if !strings.Contains(out, "no browser here") || !strings.Contains(out, "Open this URL yourself: "+url+"\n") {
		t.Errorf("stderr = %q, want a warning that names the error and the url", out)
	}
}

// TestServeIncompleteGetKeepsServing checks a browser that drops the connection mid-response doesn't use up the one load.
func TestServeIncompleteGetKeepsServing(t *testing.T) {
	page := bytes.Repeat([]byte("0123456789abcdef"), 2<<20) // 32 MiB, far above socket buffers
	stderr, err := serve(context.Background(), page, func(u string) {
		addr := strings.TrimPrefix(u[:strings.LastIndex(u, "/")], "http://")
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			t.Errorf("dial: %v", err)
			return
		}
		fmt.Fprintf(conn, "GET %s HTTP/1.1\r\nHost: %s\r\n\r\n", u[strings.LastIndex(u, "/"):], addr)
		buf := make([]byte, 512)
		if _, err := io.ReadFull(conn, buf); err != nil {
			t.Errorf("read: %v", err)
		}
		conn.(*net.TCPConn).SetLinger(0) // reset instead of a clean close
		conn.Close()

		resp, body, err := get(u)
		if err != nil || resp.StatusCode != http.StatusOK || !bytes.Equal(body, page) {
			t.Errorf("full GET after an aborted one: err %v, %d bytes", err, len(body))
		}
	})
	if err != nil {
		t.Fatalf("Serve: %v\n%s", err, stderr)
	}
}
