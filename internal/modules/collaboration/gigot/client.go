package gigot

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

func (m *Manager) do(method string, conn Connection, relPath string, query map[string]string, body any, out any) error {
	endpoint, err := buildURL(conn.BaseURL, relPath, query)
	if err != nil {
		return err
	}

	var reqBody io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("gigot: encode request body: %w", err)
		}
		reqBody = bytes.NewReader(raw)
	}

	req, err := http.NewRequest(method, endpoint, reqBody)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if conn.Token != "" {
		req.Header.Set("Authorization", "Bearer "+conn.Token)
	}

	resp, err := m.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return &HTTPError{
			Status: resp.StatusCode,
			Method: method,
			Path:   relPath,
			Body:   strings.TrimSpace(string(raw)),
		}
	}

	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil && err != io.EOF {
		return fmt.Errorf("gigot: decode response: %w", err)
	}
	return nil
}

func buildURL(baseURL, relPath string, query map[string]string) (string, error) {
	if baseURL == "" {
		return "", ErrMissingBaseURL
	}
	if !strings.HasPrefix(relPath, "/") {
		return "", fmt.Errorf("gigot: route path must start with /: %q", relPath)
	}
	base := strings.TrimRight(baseURL, "/")
	full := base + relPath
	if len(query) == 0 {
		return full, nil
	}
	u, err := url.Parse(full)
	if err != nil {
		return "", err
	}
	q := u.Query()
	for k, v := range query {
		if v == "" {
			continue
		}
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func encodeSegment(s string) string {
	return url.PathEscape(s)
}

func encodeSegments(p string) string {
	parts := strings.Split(p, "/")
	for i, seg := range parts {
		parts[i] = url.PathEscape(seg)
	}
	return strings.Join(parts, "/")
}
