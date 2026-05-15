package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/michaelquigley/df/dd"
)

type Client struct {
	baseURL    *url.URL
	token      string
	httpClient *http.Client
}

func New(cfg *Config) (*Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	baseURL, err := url.Parse(strings.TrimRight(cfg.URL, "/"))
	if err != nil {
		return nil, err
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if cfg.TLS != nil {
		tlsConfig := &tls.Config{InsecureSkipVerify: cfg.TLS.InsecureSkipVerify}
		if cfg.TLS.CACert != "" {
			pool, err := x509.SystemCertPool()
			if err != nil {
				pool = x509.NewCertPool()
			}
			raw, err := os.ReadFile(cfg.TLS.CACert)
			if err != nil {
				return nil, err
			}
			if !pool.AppendCertsFromPEM(raw) {
				return nil, fmt.Errorf("no certificates found in %s", cfg.TLS.CACert)
			}
			tlsConfig.RootCAs = pool
		}
		transport.TLSClientConfig = tlsConfig
	}
	return &Client{
		baseURL: baseURL,
		token:   cfg.Token,
		httpClient: &http.Client{
			Transport: transport,
		},
	}, nil
}

func (c *Client) DoJSON(ctx context.Context, method string, path string, body any, out any) error {
	raw, err := c.do(ctx, method, path, body)
	if err != nil {
		return err
	}
	if out == nil || len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	switch v := out.(type) {
	case *map[string]any:
		return json.Unmarshal(raw, v)
	case *[]map[string]any:
		return json.Unmarshal(raw, v)
	}
	if err := dd.BindJSON(out, raw); err != nil {
		return err
	}
	return nil
}

func (c *Client) DoText(ctx context.Context, method string, path string, body any) (string, error) {
	raw, err := c.do(ctx, method, path, body)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func (c *Client) do(ctx context.Context, method string, path string, body any) ([]byte, error) {
	req, err := c.request(ctx, method, path, body)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s %s: %s: %s", method, path, resp.Status, strings.TrimSpace(string(raw)))
	}
	return raw, nil
}

func (c *Client) request(ctx context.Context, method string, path string, body any) (*http.Request, error) {
	rel, err := url.Parse(path)
	if err != nil {
		return nil, err
	}
	u := *c.baseURL
	u.Path = strings.TrimRight(c.baseURL.Path, "/") + "/" + strings.TrimLeft(rel.Path, "/")
	u.RawQuery = rel.RawQuery
	var reader io.Reader
	if body != nil {
		raw, err := encodeBody(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

func encodeBody(body any) ([]byte, error) {
	switch v := body.(type) {
	case []byte:
		return v, nil
	case json.RawMessage:
		return v, nil
	case map[string]any:
		return json.Marshal(v)
	default:
		return dd.UnbindJSON(body)
	}
}
