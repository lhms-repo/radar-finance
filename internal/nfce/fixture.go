package nfce

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// fixtureName is the file part of a fixture address: letters, digits and dashes only.
var fixtureName = regexp.MustCompile(`^[a-z0-9-]{1,40}$`)

// FixtureClient reads fictional receipts from a folder instead of the portal; end to end tests use it.
// The p parameter is "file|access key|cnpj", so each test gets its own receipt from the same page.
type FixtureClient struct {
	Dir string
}

// Fetch parses the fixture named in the address and applies the access key and CNPJ it carries.
func (c FixtureClient) Fetch(_ context.Context, raw string) (Receipt, error) {
	u, err := CheckURL(raw)
	if err != nil {
		return Receipt{}, err
	}
	parts := strings.Split(u.Query().Get("p"), "|")
	if !fixtureName.MatchString(parts[0]) {
		return Receipt{}, ErrNotReceiptURL
	}
	f, err := os.Open(filepath.Join(c.Dir, parts[0]+".html"))
	if err != nil {
		return Receipt{}, ErrNotReceiptURL
	}
	defer f.Close()
	rec, err := Parse(f)
	if err != nil {
		return Receipt{}, ErrLayout
	}
	if len(parts) > 1 && len(digits(parts[1])) == 44 {
		rec.AccessKey = digits(parts[1])
	}
	if len(parts) > 2 && len(digits(parts[2])) == 14 {
		rec.StoreCNPJ = digits(parts[2])
	}
	return rec, nil
}
