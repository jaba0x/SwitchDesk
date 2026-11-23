package cxrest

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

type fakeCX struct {
	mu       sync.Mutex
	got      []byte
	bank     string
	name     string
	loggedIn bool
	logouts  int
	noLatest bool
}

func (f *fakeCX) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/rest", func(w http.ResponseWriter, r *http.Request) {
		if f.noLatest {
			http.NotFound(w, r)
			return
		}
		io.WriteString(w, `{"latest":{"version":"10.13","prefix":"/rest/v10.13"},"v10.09":{"version":"10.09","prefix":"/rest/v10.09"}}`)
	})
	mux.HandleFunc("/rest/v10.13/login", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		if r.Form.Get("username") != "admin" || r.Form.Get("password") != "pw" {
			w.WriteHeader(401)
			return
		}
		if r.Header.Get("x-use-csrf-token") == "true" {
			w.Header().Set("X-Csrf-Token", "tok123")
		}
		http.SetCookie(w, &http.Cookie{Name: "id", Value: "sess", Path: "/"})
		f.mu.Lock()
		f.loggedIn = true
		f.mu.Unlock()
	})
	mux.HandleFunc("/rest/v10.13/logout", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.logouts++
		f.mu.Unlock()
	})
	mux.HandleFunc("/rest/v10.13/firmware", func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie("id"); err != nil || c.Value != "sess" || r.Header.Get("x-csrf-token") != "tok123" {
			w.WriteHeader(401)
			return
		}
		file, hdr, err := r.FormFile("fileupload")
		if err != nil {
			w.WriteHeader(400)
			io.WriteString(w, "Error processing the form.")
			return
		}
		data, _ := io.ReadAll(file)
		f.mu.Lock()
		f.got, f.bank, f.name = data, r.URL.Query().Get("image"), hdr.Filename
		f.mu.Unlock()
	})
	return mux
}

func TestPush(t *testing.T) {
	f := &fakeCX{}
	srv := httptest.NewTLSServer(f.handler())
	defer srv.Close()
	img := bytes.Repeat([]byte("firmware"), 100000)
	var last int64
	err := Push(context.Background(), srv.URL, nil, "admin", "pw", "secondary", "/lib/PL_10_16_1060.swi", bytes.NewReader(img), int64(len(img)), func(s, _ int64) { last = s })
	if err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if !bytes.Equal(f.got, img) || f.bank != "secondary" || f.name != "PL_10_16_1060.swi" {
		t.Fatalf("switch got %d bytes into %q as %q", len(f.got), f.bank, f.name)
	}
	if last != int64(len(img)) {
		t.Fatalf("progress ended at %d", last)
	}
	if f.logouts != 1 {
		t.Fatalf("logouts: %d", f.logouts)
	}
}

func TestPushBadPassword(t *testing.T) {
	srv := httptest.NewTLSServer((&fakeCX{}).handler())
	defer srv.Close()
	err := Push(context.Background(), srv.URL, nil, "admin", "nope", "secondary", "x.swi", strings.NewReader("x"), 1, nil)
	if err == nil || !strings.Contains(err.Error(), "refused the login") {
		t.Fatalf("got %v", err)
	}
}

func TestPushNoVersionList(t *testing.T) {
	// older releases have no /rest index; login walks the known versions
	f := &fakeCX{noLatest: true}
	srv := httptest.NewTLSServer(f.handler())
	defer srv.Close()
	if err := Push(context.Background(), srv.URL, nil, "admin", "pw", "primary", "x.swi", strings.NewReader("abc"), 3, nil); err != nil {
		t.Fatal(err)
	}
	if string(f.got) != "abc" || f.bank != "primary" {
		t.Fatalf("got %q into %q", f.got, f.bank)
	}
}

func TestNoREST(t *testing.T) {
	err := Push(context.Background(), "https://127.0.0.1:1", nil, "a", "b", "primary", "x", strings.NewReader(""), 0, nil)
	if err == nil || !strings.Contains(err.Error(), "REST API didn't answer") {
		t.Fatalf("got %v", err)
	}
}

// Issue #46: AOS-CX 10.16 answers an upload without a Content-Length with
// 400 Bad Request. The upload now says how long it is.
func TestUploadSendsContentLength(t *testing.T) {
	f := &fakeCX{}
	inner := f.handler()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/firmware") && (r.ContentLength <= 0 || len(r.TransferEncoding) > 0) {
			w.WriteHeader(400)
			io.WriteString(w, "Bad Request")
			return
		}
		inner.ServeHTTP(w, r)
	}))
	defer srv.Close()
	img := bytes.Repeat([]byte("cx-image"), 50000)
	if err := Push(context.Background(), srv.URL, nil, "admin", "pw", "secondary", "AOS-CX_6200_10_16_1051.swi", bytes.NewReader(img), int64(len(img)), nil); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if !bytes.Equal(f.got, img) || f.name != "AOS-CX_6200_10_16_1051.swi" {
		t.Fatalf("switch got %d bytes as %q", len(f.got), f.name)
	}
}

// If the newest REST version still refuses the upload with 400, the older
// ones the switch offers are tried with the same file.
func TestUploadFallsBackToOlderREST(t *testing.T) {
	f := &fakeCX{}
	inner := f.handler()
	tried := []string{}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/rest":
			io.WriteString(w, `{"latest":{"version":"10.16","prefix":"/rest/v10.16"},"v10.13":{"version":"10.13","prefix":"/rest/v10.13"},"v10.09":{"version":"10.09","prefix":"/rest/v10.09"}}`)
		case r.URL.Path == "/rest/v10.16/login":
			r.URL.Path = "/rest/v10.13/login"
			inner.ServeHTTP(w, r)
		case strings.HasSuffix(r.URL.Path, "/firmware"):
			tried = append(tried, r.URL.Path)
			if r.URL.Path != "/rest/v10.13/firmware" {
				io.Copy(io.Discard, r.Body)
				w.WriteHeader(400)
				return
			}
			inner.ServeHTTP(w, r)
		default:
			inner.ServeHTTP(w, r)
		}
	}))
	defer srv.Close()
	img := bytes.Repeat([]byte("x"), 300000)
	if err := Push(context.Background(), srv.URL, nil, "admin", "pw", "primary", "a.swi", bytes.NewReader(img), int64(len(img)), nil); err != nil {
		t.Fatal(err)
	}
	if strings.Join(tried, " ") != "/rest/v10.16/firmware /rest/v10.13/firmware" {
		t.Fatalf("tried %v", tried)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if !bytes.Equal(f.got, img) {
		t.Fatalf("switch got %d bytes", len(f.got))
	}
	if !versionLess("/rest/v10.09", "/rest/v10.13") || versionLess("/rest/v10.16", "/rest/v1") {
		t.Fatal("versionLess")
	}
}
