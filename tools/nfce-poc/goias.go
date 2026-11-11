package main

import (
	"bytes"
	"errors"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// The Goiás portal answers the QR code URL with a shell page whose iframe,
// fetched with the same session cookie, carries the receipt as a JavaScript
// string passed to `new DanfeNFCe(container, images, html)`.

// iframePattern finds the src of the receipt iframe in the shell page.
var iframePattern = regexp.MustCompile(`<iframe[^>]*class="iframe-danfe-nfce"[^>]*src="([^"]+)"`)

// errNoEmbedded is returned when the frame does not call DanfeNFCe with the receipt HTML.
var errNoEmbedded = errors.New("receipt HTML not found inside new DanfeNFCe(...)")

// frameURL returns the absolute URL of the receipt iframe, if the page is the shell.
func frameURL(page []byte, base *url.URL) (string, bool) {
	m := iframePattern.FindSubmatch(page)
	if m == nil {
		return "", false
	}
	ref, err := url.Parse(string(m[1]))
	if err != nil {
		return "", false
	}
	return base.ResolveReference(ref).String(), true
}

// embeddedReceipt extracts and unescapes the third argument of new DanfeNFCe(...).
func embeddedReceipt(frame []byte) ([]byte, error) {
	i := bytes.Index(frame, []byte("new DanfeNFCe("))
	if i < 0 {
		return nil, errNoEmbedded
	}
	rest := string(frame[i+len("new DanfeNFCe("):])
	var args []string
	for len(args) < 3 {
		start := strings.IndexByte(rest, '\'')
		if start < 0 {
			return nil, errNoEmbedded
		}
		s, n, err := readJSString(rest[start:])
		if err != nil {
			return nil, err
		}
		args = append(args, s)
		rest = rest[start+n:]
	}
	return []byte(args[2]), nil
}

// readJSString decodes a single quoted JavaScript literal at the start of s, returning it and the bytes consumed.
func readJSString(s string) (string, int, error) {
	var b strings.Builder
	for i := 1; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\'':
			return b.String(), i + 1, nil
		case c == '\\' && i+1 < len(s):
			i++
			switch e := s[i]; e {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			case 'u':
				if i+4 >= len(s) {
					return "", 0, errNoEmbedded
				}
				r, err := strconv.ParseUint(s[i+1:i+5], 16, 32)
				if err != nil {
					return "", 0, errNoEmbedded
				}
				b.WriteRune(rune(r))
				i += 4
			default:
				// \" \' \/ \\ and anything else stand for the character itself.
				b.WriteByte(e)
			}
		default:
			b.WriteByte(c)
		}
	}
	return "", 0, errNoEmbedded
}
