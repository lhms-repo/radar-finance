// Command nfce-poc is the isolated proof of concept for reading a Goiás NFC-e
// from the QR code URL (phase 1, prepares RF-68). It is not part of the binary.
//
// Usage:
//
//	go run ./tools/nfce-poc -url 'https://nfeweb.sefaz.go.gov.br/nfeweb/sites/nfce/danfeNFCe?p=...'
//	go run ./tools/nfce-poc -file tools/nfce-poc/samples/nota.html
//
// With -save, the raw page is written to tools/nfce-poc/samples, which is
// ignored by git: real receipts never go to the repository.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// userAgent mimics Safari on iPhone, the device that scans the QR code.
const userAgent = "Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1"

// main fetches or reads a receipt page and prints what could be parsed.
func main() {
	rawURL := flag.String("url", "", "URL lida do QR code da NFC-e")
	file := flag.String("file", "", "arquivo HTML salvo antes, para analisar sem rede")
	save := flag.Bool("save", false, "salva o HTML bruto em tools/nfce-poc/samples")
	flag.Parse()

	var body []byte
	var err error
	switch {
	case *file != "":
		body, err = os.ReadFile(*file)
	case *rawURL != "":
		body, err = fetch(*rawURL)
		if err == nil && *save {
			if path, saveErr := saveSample(body); saveErr == nil {
				fmt.Fprintln(os.Stderr, "HTML salvo em", path)
			} else {
				fmt.Fprintln(os.Stderr, "não foi possível salvar:", saveErr)
			}
		}
	default:
		flag.Usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "erro:", err)
		os.Exit(1)
	}

	rec, err := Parse(bytes.NewReader(body))
	out, _ := json.MarshalIndent(rec, "", "  ")
	fmt.Println(string(out))
	if err != nil {
		fmt.Fprintln(os.Stderr, "aviso:", err)
		if errors.Is(err, ErrNoItems) {
			fmt.Fprintln(os.Stderr, "Rode de novo com -save e analise o HTML salvo para ajustar o parser.")
		}
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "%d itens lidos.\n", len(rec.Items))
}

// fetch downloads the receipt page, keeping cookies across redirects.
func fetch(raw string) ([]byte, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("URL inválida: %w", err)
	}
	if !strings.HasSuffix(u.Hostname(), "sefaz.go.gov.br") {
		fmt.Fprintln(os.Stderr, "aviso: a URL não é da Sefaz de Goiás:", u.Hostname())
	}
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Timeout: 30 * time.Second, Jar: jar}
	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	req.Header.Set("Accept-Language", "pt-BR,pt;q=0.9")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 5<<20))
	if err != nil {
		return nil, err
	}
	fmt.Fprintf(os.Stderr, "HTTP %d, %d bytes, URL final %s\n", resp.StatusCode, len(body), resp.Request.URL)
	if resp.StatusCode != http.StatusOK {
		return body, fmt.Errorf("a Sefaz respondeu %d", resp.StatusCode)
	}
	return body, nil
}

// saveSample writes the raw page to the git ignored samples folder.
func saveSample(body []byte) (string, error) {
	dir := filepath.Join("tools", "nfce-poc", "samples")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", err
	}
	path := filepath.Join(dir, time.Now().Format("20060102-150405")+".html")
	return path, os.WriteFile(path, body, 0o600)
}
