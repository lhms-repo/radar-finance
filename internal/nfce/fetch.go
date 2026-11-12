package nfce

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
	"unicode"

	"github.com/italogfx/radar-finance/internal/problem"
)

// userAgent mimics Safari on iPhone, the device that scans the QR code.
const userAgent = "Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1"

// Errors shown to the user when a receipt cannot be read.
var (
	ErrNotReceiptURL = problem.Invalid("receipt_url_invalid", "Esse link não é de uma nota fiscal da Sefaz. Leia o QR code da NFC-e de novo.")
	ErrPortal        = problem.Invalid("receipt_portal_failed", "A Sefaz não respondeu agora. Tente de novo em alguns minutos.")
	ErrLayout        = problem.Invalid("receipt_layout_unknown", "Não consegui ler os itens dessa nota. O site da Sefaz pode ter mudado.")
)

// upgradeTimeout bounds the https attempt of an http:// QR code, so a portal without https costs little.
const upgradeTimeout = 10 * time.Second

// Client downloads receipts from the state portals.
type Client struct {
	timeout   time.Duration
	allowHost func(host string) bool
	// transport replaces the default one in tests; nil uses Go defaults.
	transport http.RoundTripper
}

// NewClient returns a client that only talks to government hosts.
func NewClient() *Client {
	return &Client{timeout: 30 * time.Second, allowHost: isGovHost}
}

// normalize drops the line breaks, control and invisible characters some QR codes carry inside the address,
// which a valid address never has, and adds https:// when the code brings only the host and path.
func normalize(raw string) string {
	raw = strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, raw)
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	return raw
}

// CheckURL accepts only http or https addresses of government hosts, so the server never fetches other machines.
func CheckURL(raw string) (*url.URL, error) {
	u, err := url.Parse(normalize(raw))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || !isGovHost(u.Hostname()) {
		return nil, ErrNotReceiptURL
	}
	return u, nil
}

// isGovHost reports whether the host belongs to the Brazilian government domain.
func isGovHost(host string) bool {
	host = strings.ToLower(host)
	return strings.HasSuffix(host, ".gov.br")
}

// Fetch downloads and parses the receipt (RF-68). An http:// QR code is read over https first, so nobody
// on the way can swap the items and prices, and over http only when the portal does not answer there.
func (c *Client) Fetch(ctx context.Context, raw string) (Receipt, error) {
	u, err := url.Parse(normalize(raw))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || !c.allowHost(u.Hostname()) {
		return Receipt{}, ErrNotReceiptURL
	}
	if u.Scheme == "http" {
		secure := *u
		secure.Scheme = "https"
		upgradeCtx, cancel := context.WithTimeout(ctx, upgradeTimeout)
		rec, err := c.fetch(upgradeCtx, &secure)
		cancel()
		if err == nil {
			return rec, nil
		}
	}
	return c.fetch(ctx, u)
}

// fetch downloads one address; on the Goiás shell page it follows the iframe with the same cookies and
// unwraps the HTML embedded in the frame script.
func (c *Client) fetch(ctx context.Context, u *url.URL) (Receipt, error) {
	jar, _ := cookiejar.New(nil)
	client := &http.Client{
		Timeout:   c.timeout,
		Jar:       jar,
		Transport: c.transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 || !c.allowHost(req.URL.Hostname()) {
				return ErrNotReceiptURL
			}
			return nil
		},
	}
	page, final, err := c.get(ctx, client, u.String(), "")
	if err != nil {
		return Receipt{}, err
	}
	if frame, ok := frameURL(page, final); ok {
		fu, err := url.Parse(frame)
		if err != nil || !c.allowHost(fu.Hostname()) {
			return Receipt{}, ErrNotReceiptURL
		}
		body, _, err := c.get(ctx, client, frame, final.String())
		if err != nil {
			return Receipt{}, err
		}
		if page, err = embeddedReceipt(body); err != nil {
			return Receipt{}, ErrLayout
		}
	}
	rec, err := Parse(bytes.NewReader(page))
	if err != nil {
		return Receipt{}, ErrLayout
	}
	return rec, nil
}

// get downloads one page with the iPhone headers, returning the body and the final URL.
func (c *Client) get(ctx context.Context, client *http.Client, target, referer string) ([]byte, *url.URL, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, nil, ErrNotReceiptURL
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	req.Header.Set("Accept-Language", "pt-BR,pt;q=0.9")
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	resp, err := client.Do(req)
	if err != nil {
		if errors.Is(err, ErrNotReceiptURL) {
			return nil, nil, ErrNotReceiptURL
		}
		return nil, nil, fmt.Errorf("%w: %w", ErrPortal, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 5<<20))
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %w", ErrPortal, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("%w: status %d", ErrPortal, resp.StatusCode)
	}
	return body, resp.Request.URL, nil
}
