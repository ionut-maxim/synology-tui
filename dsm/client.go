// Package dsm is a small client for the Synology DSM 7 Web API (entry.cgi):
// system, storage, S.M.A.R.T., packages and logs.
//
// DSM documents only a few of its APIs. The method names and parameters here
// come from DSM's own web UI code (DSM 7.4); see docs/dsm-api.md.
package dsm

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Client is one DSM session. It is safe for concurrent use.
type Client struct {
	baseURL string
	http    *http.Client
	session string

	mu       sync.Mutex
	sid      string
	user     string
	password string
}

// Options configure a Client.
type Options struct {
	// Insecure skips TLS verification: DSM ships a self-signed certificate.
	Insecure bool
	// Timeout per request (default 30s). Package start/stop can take longer;
	// those calls pass their own context deadline.
	Timeout time.Duration
	// Session is the DSM session name (default "synology-tui").
	Session string
}

// New returns a client for baseURL, e.g. "https://nas.local:5001".
func New(baseURL string, opts Options) *Client {
	if opts.Timeout == 0 {
		opts.Timeout = 30 * time.Second
	}
	if opts.Session == "" {
		opts.Session = "synology-tui"
	}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	if opts.Insecure {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // DSM's default certificate is self-signed
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: opts.Timeout, Transport: tr},
		session: opts.Session,
	}
}

type envelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   *struct {
		Code int `json:"code"`
	} `json:"error"`
}

// Login opens a session (SYNO.API.Auth v7). The credentials are kept so an
// expired session is renewed transparently.
func (c *Client) Login(ctx context.Context, user, password string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.user, c.password = user, password
	return c.loginLocked(ctx)
}

func (c *Client) loginLocked(ctx context.Context) error {
	form := url.Values{
		"api": {"SYNO.API.Auth"}, "version": {"7"}, "method": {"login"},
		"account": {c.user}, "passwd": {c.password},
		"session": {c.session}, "format": {"sid"},
	}
	var out struct {
		SID string `json:"sid"`
	}
	if err := c.post(ctx, form, "SYNO.API.Auth", "login", &out); err != nil {
		return err
	}
	if out.SID == "" {
		return errors.New("dsm: login returned no session id")
	}
	c.sid = out.SID
	return nil
}

// Logout ends the session.
func (c *Client) Logout(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sid == "" {
		return nil
	}
	form := url.Values{
		"api": {"SYNO.API.Auth"}, "version": {"7"}, "method": {"logout"},
		"session": {c.session}, "_sid": {c.sid},
	}
	c.sid = ""
	return c.post(ctx, form, "SYNO.API.Auth", "logout", nil)
}

// Call invokes api/method with params and decodes "data" into out (may be
// nil). A session that DSM expired is renewed once and the call retried.
func (c *Client) Call(ctx context.Context, api string, version int, method string, params url.Values, out any) error {
	for attempt := 0; ; attempt++ {
		c.mu.Lock()
		sid := c.sid
		c.mu.Unlock()
		if sid == "" {
			return errors.New("dsm: not logged in")
		}
		form := url.Values{"api": {api}, "version": {strconv.Itoa(version)}, "method": {method}, "_sid": {sid}}
		for k, v := range params {
			form[k] = v
		}
		err := c.post(ctx, form, api, method, out)
		var apiErr *Error
		if attempt == 0 && errors.As(err, &apiErr) && apiErr.SessionExpired() {
			c.mu.Lock()
			if c.sid == sid { // nobody renewed it meanwhile
				err = c.loginLocked(ctx)
			}
			c.mu.Unlock()
			if err != nil {
				return err
			}
			continue
		}
		return err
	}
}

func (c *Client) post(ctx context.Context, form url.Values, api, method string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/webapi/entry.cgi", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("dsm: %s.%s: %w", api, method, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return fmt.Errorf("dsm: %s.%s: %w", api, method, err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("dsm: %s.%s: HTTP %d", api, method, resp.StatusCode)
	}
	var env envelope
	if err := json.Unmarshal(body, &env); err != nil {
		return fmt.Errorf("dsm: %s.%s: bad response: %w", api, method, err)
	}
	if !env.Success {
		code := 100
		if env.Error != nil {
			code = env.Error.Code
		}
		return &Error{API: api, Method: method, Code: code}
	}
	if out == nil || len(env.Data) == 0 {
		return nil
	}
	if err := json.Unmarshal(env.Data, out); err != nil {
		return fmt.Errorf("dsm: %s.%s: decoding data: %w", api, method, err)
	}
	return nil
}

// jsonParam encodes v as a JSON request parameter (DSM wants lists as JSON).
func jsonParam(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
