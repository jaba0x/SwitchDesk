// Package cxrest uploads firmware images to AOS-CX switches through their
// REST API. AOS-CX has no SFTP or SCP server, so this is the only way to send
// an image to the switch instead of having the switch fetch it.
package cxrest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/textproto"
	"net/url"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jaba0x/switchdesk/internal/tlspin"
)

// Fallback versions tried when GET /rest doesn't say which ones exist.
var knownVersions = []string{"v10.13", "v10.12", "v10.11", "v10.10", "v10.09", "v10.08", "v10.04", "v1"}

// ErrNoREST means nothing answered on HTTPS, or nothing that looks like the REST API.
var ErrNoREST = errors.New("the switch's REST API didn't answer on HTTPS")

type Client struct {
	Base string // https://host[:port]
	HTTP *http.Client
	pre  string   // /rest/v10.xx
	alts []string // the other REST versions the switch offers, newest first
	csrf string   // returned by login on releases that use it
}

// New returns a client for base. Switches use self-signed certificates, so
// instead of a CA check the certificate must match pin (or, with an empty
// pin.Want, is remembered in pin for the caller to store). A nil pin trusts
// the first certificate it sees.
func New(base string, pin *tlspin.Pin) *Client {
	if pin == nil {
		pin = &tlspin.Pin{}
	}
	jar, _ := cookiejar.New(nil)
	return &Client{Base: strings.TrimRight(base, "/"), HTTP: &http.Client{
		Jar:       jar,
		Transport: &http.Transport{TLSClientConfig: pin.Config(), Proxy: nil, ResponseHeaderTimeout: 20 * time.Minute},
	}}
}

func (c *Client) do(ctx context.Context, method, path string, body io.Reader, ctype string, timeout time.Duration) (*http.Response, error) {
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer func() {
			if cancel != nil {
				cancel()
			}
		}()
	}
	req, err := http.NewRequestWithContext(ctx, method, c.Base+path, body)
	if err != nil {
		return nil, err
	}
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	c.headers(req)
	if strings.HasSuffix(path, "/login") {
		req.Header.Set("x-use-csrf-token", "true")
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	if t := resp.Header.Get("X-Csrf-Token"); t != "" {
		c.csrf = t
	}
	// read the body now so the timeout context can be released
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()
	resp.Body = io.NopCloser(strings.NewReader(string(data)))
	return resp, err
}

func (c *Client) headers(req *http.Request) {
	req.Header.Set("Accept", "*/*")
	if c.csrf != "" {
		req.Header.Set("x-csrf-token", c.csrf)
	}
}

func bodyText(r *http.Response) string {
	b, _ := io.ReadAll(r.Body)
	t := strings.TrimSpace(string(b))
	if len(t) > 200 {
		t = t[:200]
	}
	return t
}

// version finds the newest REST version the switch offers.
func (c *Client) version(ctx context.Context) error {
	resp, err := c.do(ctx, "GET", "/rest", nil, "", 20*time.Second)
	if errors.Is(err, tlspin.ErrChanged) {
		return tlspin.ErrChanged
	}
	if err != nil {
		return fmt.Errorf("%w (%v)", ErrNoREST, err)
	}
	var v map[string]struct {
		Prefix string `json:"prefix"`
	}
	if resp.StatusCode == 200 && json.NewDecoder(resp.Body).Decode(&v) == nil && strings.HasPrefix(v["latest"].Prefix, "/rest/") {
		c.pre = v["latest"].Prefix
		for k, x := range v {
			if k != "latest" && strings.HasPrefix(x.Prefix, "/rest/") && x.Prefix != c.pre {
				c.alts = append(c.alts, x.Prefix)
			}
		}
		sort.Slice(c.alts, func(i, j int) bool { return versionLess(c.alts[j], c.alts[i]) })
		return nil
	}
	if resp.StatusCode == 404 || resp.StatusCode == 200 {
		c.pre = "" // unknown; login tries the known versions
		return nil
	}
	return fmt.Errorf("%w (HTTP %d)", ErrNoREST, resp.StatusCode)
}

// Login opens a REST session. Call Logout when done: switches allow only a
// few sessions per user.
func (c *Client) Login(ctx context.Context, user, pass string) error {
	if err := c.version(ctx); err != nil {
		return err
	}
	try := knownVersions
	if c.pre != "" {
		try = []string{strings.TrimPrefix(c.pre, "/rest/")}
	}
	form := url.Values{"username": {user}, "password": {pass}}.Encode()
	var last string
	for _, v := range try {
		resp, err := c.do(ctx, "POST", "/rest/"+v+"/login", strings.NewReader(form), "application/x-www-form-urlencoded", 30*time.Second)
		if err != nil {
			return fmt.Errorf("%w (%v)", ErrNoREST, err)
		}
		switch resp.StatusCode {
		case 200, 201, 204:
			c.pre = "/rest/" + v
			return nil
		case 401, 403:
			return fmt.Errorf("the switch's REST API refused the login (HTTP %d). The SSH account needs administrator rights", resp.StatusCode)
		case 429, 503:
			return fmt.Errorf("the switch has too many REST sessions open (HTTP %d); wait a few minutes or log out of its web page", resp.StatusCode)
		}
		last = fmt.Sprintf("HTTP %d %s", resp.StatusCode, bodyText(resp))
	}
	return fmt.Errorf("%w: login failed (%s)", ErrNoREST, last)
}

func (c *Client) Logout(ctx context.Context) {
	if c.pre == "" {
		return
	}
	c.do(ctx, "POST", c.pre+"/logout", nil, "", 15*time.Second)
}

// Upload sends the image to bank and returns once the switch has written and
// checked it. progress reports bytes sent.
//
// The request carries a Content-Length, like a browser's or curl's upload:
// AOS-CX 10.16 answers a chunked upload with "400 Bad Request" (issue #46).
// If the switch still says 400 and the file can be read again, the upload is
// retried on the older REST versions the switch offers.
func (c *Client) Upload(ctx context.Context, bank, name string, file io.Reader, size int64, progress func(sent, total int64)) error {
	err := c.upload(ctx, c.pre, bank, name, file, size, progress)
	seeker, canRewind := file.(io.Seeker)
	for _, pre := range c.alts {
		var bad *badRequest
		if !errors.As(err, &bad) || !canRewind {
			break
		}
		if _, serr := seeker.Seek(0, io.SeekStart); serr != nil {
			break
		}
		if e := c.upload(ctx, pre, bank, name, file, size, progress); e == nil {
			return nil
		} else if !errors.As(e, &bad) {
			return e
		}
	}
	return err
}

type badRequest struct{ text string }

func (e *badRequest) Error() string { return e.text }

func (c *Client) upload(ctx context.Context, pre, bank, name string, file io.Reader, size int64, progress func(sent, total int64)) error {
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="fileupload"; filename="%s"`, strings.ReplaceAll(filepath.Base(name), `"`, "")))
	h.Set("Content-Type", "application/octet-stream")
	var body io.Reader
	var length int64 = -1
	var ctype string
	if size > 0 {
		// the multipart framing around the file is known up front, so the
		// whole length can be sent
		var head, tail bytes.Buffer
		mw := multipart.NewWriter(&head)
		if _, err := mw.CreatePart(h); err != nil {
			return err
		}
		ctype = mw.FormDataContentType()
		prefix := append([]byte(nil), head.Bytes()...)
		head.Reset()
		mw2 := multipart.NewWriter(&tail)
		mw2.SetBoundary(mw.Boundary())
		mw2.Close()
		body = io.MultiReader(bytes.NewReader(prefix), &counter{r: io.LimitReader(file, size), total: size, fn: progress}, &tail)
		length = int64(len(prefix)) + size + int64(tail.Len())
	} else {
		pr, pw := io.Pipe()
		mw := multipart.NewWriter(pw)
		ctype = mw.FormDataContentType()
		go func() {
			part, err := mw.CreatePart(h)
			if err == nil {
				_, err = io.Copy(part, &counter{r: file, total: size, fn: progress})
			}
			if err == nil {
				err = mw.Close()
			}
			pw.CloseWithError(err)
		}()
		body = pr
	}
	req, err := http.NewRequestWithContext(ctx, "POST", c.Base+pre+"/firmware?image="+url.QueryEscape(bank), body)
	if err != nil {
		return err
	}
	if length >= 0 {
		req.ContentLength = length
	}
	c.headers(req)
	req.Header.Set("Content-Type", ctype)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("uploading over HTTPS: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 400 {
		return &badRequest{fmt.Sprintf("the switch didn't accept the image (HTTP 400 on %s): %s", strings.TrimPrefix(pre, "/rest/"), bodyText(resp))}
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("the switch didn't accept the image (HTTP %d): %s", resp.StatusCode, bodyText(resp))
	}
	return nil
}

// versionLess orders /rest/v10.09 before /rest/v10.13, and v1 before both.
func versionLess(a, b string) bool {
	num := func(p string) []int {
		var out []int
		for _, f := range strings.FieldsFunc(strings.TrimPrefix(p, "/rest/v"), func(r rune) bool { return r == '.' }) {
			n, _ := strconv.Atoi(f)
			out = append(out, n)
		}
		return out
	}
	x, y := num(a), num(b)
	for i := 0; i < len(x) && i < len(y); i++ {
		if x[i] != y[i] {
			return x[i] < y[i]
		}
	}
	return len(x) < len(y)
}

type counter struct {
	r     io.Reader
	sent  int64
	total int64
	last  time.Time
	fn    func(sent, total int64)
}

func (c *counter) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.sent += int64(n)
	if c.fn != nil && (time.Since(c.last) > 300*time.Millisecond || err == io.EOF) {
		c.last = time.Now()
		c.fn(c.sent, c.total)
	}
	return n, err
}

// Push logs in, uploads and logs out.
func Push(ctx context.Context, base string, pin *tlspin.Pin, user, pass, bank, name string, file io.Reader, size int64, progress func(sent, total int64)) error {
	c := New(base, pin)
	if err := c.Login(ctx, user, pass); err != nil {
		return err
	}
	defer c.Logout(context.Background())
	return c.Upload(ctx, bank, name, file, size, progress)
}
