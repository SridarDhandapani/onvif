package onvif

import (
	"crypto/md5"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// GetSnapshotUri returns the HTTP(S) URL of a still JPEG for the given media
// profile. Mirrors GetStreamUri.
func (c *Client) GetSnapshotUri(camera *Camera, profileToken string) (string, error) {
	mediaURL := c.resolveMediaURL(camera)
	body := fmt.Sprintf(`<trt:GetSnapshotUri><trt:ProfileToken>%s</trt:ProfileToken></trt:GetSnapshotUri>`, profileToken)
	resp, err := c.sendSOAPRequest(mediaURL,
		"http://www.onvif.org/ver10/media/wsdl/GetSnapshotUri", body)
	if err != nil {
		return "", fmt.Errorf("failed to get snapshot URI: %v", err)
	}
	if err := parseSOAPFault(resp); err != nil {
		return "", err
	}

	var parsed struct {
		Uri string `xml:"Body>GetSnapshotUriResponse>MediaUri>Uri"`
	}
	if err := xml.Unmarshal(resp, &parsed); err != nil {
		return "", fmt.Errorf("failed to parse snapshot URI: %v", err)
	}
	if parsed.Uri == "" {
		return "", fmt.Errorf("no snapshot URI in response")
	}
	return parsed.Uri, nil
}

// FetchSnapshot fetches a still JPEG for the given profile and returns the raw
// bytes. The snapshot URL typically uses HTTP Basic or Digest auth (not
// WS-Security), so this tries unauthenticated first, then satisfies whichever
// challenge the camera returns.
func (c *Client) FetchSnapshot(camera *Camera, profileToken string) ([]byte, error) {
	uri, err := c.GetSnapshotUri(camera, profileToken)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: c.Timeout}
	if c.InsecureTLS {
		client.Transport = &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
	}

	// First attempt: no auth (also reveals the auth challenge if required).
	req, err := http.NewRequest(http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("snapshot request failed: %v", err)
	}
	if resp.StatusCode == http.StatusUnauthorized && c.Username != "" {
		challenge := resp.Header.Get("WWW-Authenticate")
		_ = resp.Body.Close()

		req2, err := http.NewRequest(http.MethodGet, uri, nil)
		if err != nil {
			return nil, err
		}
		if strings.HasPrefix(strings.ToLower(challenge), "digest") {
			req2.Header.Set("Authorization", digestAuthHeader(challenge, http.MethodGet, uri, c.Username, c.Password))
		} else {
			req2.SetBasicAuth(c.Username, c.Password)
		}
		resp, err = client.Do(req2)
		if err != nil {
			return nil, fmt.Errorf("authenticated snapshot request failed: %v", err)
		}
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("snapshot HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// digestAuthHeader builds an HTTP Digest Authorization header value for the
// given challenge. Supports MD5 with optional qop=auth.
func digestAuthHeader(challenge, method, uri, username, password string) string {
	p := parseDigestChallenge(challenge)
	realm, nonce, qop, opaque := p["realm"], p["nonce"], p["qop"], p["opaque"]

	// Request URI path+query as the digest URI.
	digestURI := uri
	if i := strings.Index(uri, "://"); i != -1 {
		if j := strings.Index(uri[i+3:], "/"); j != -1 {
			digestURI = uri[i+3+j:]
		} else {
			digestURI = "/"
		}
	}

	ha1 := md5hex(username + ":" + realm + ":" + password)
	ha2 := md5hex(method + ":" + digestURI)

	var response, cnonce, nc string
	if strings.Contains(qop, "auth") {
		qop = "auth"
		cnonce = randomHex(8)
		nc = "00000001"
		response = md5hex(strings.Join([]string{ha1, nonce, nc, cnonce, qop, ha2}, ":"))
	} else {
		qop = ""
		response = md5hex(ha1 + ":" + nonce + ":" + ha2)
	}

	var b strings.Builder
	fmt.Fprintf(&b, `Digest username=%q, realm=%q, nonce=%q, uri=%q, response=%q`,
		username, realm, nonce, digestURI, response)
	if qop != "" {
		fmt.Fprintf(&b, `, qop=%s, nc=%s, cnonce=%q`, qop, nc, cnonce)
	}
	if opaque != "" {
		fmt.Fprintf(&b, `, opaque=%q`, opaque)
	}
	return b.String()
}

// parseDigestChallenge parses the comma-separated key="value" pairs of a Digest
// WWW-Authenticate header.
func parseDigestChallenge(challenge string) map[string]string {
	out := map[string]string{}
	challenge = strings.TrimSpace(challenge)
	challenge = strings.TrimPrefix(challenge, "Digest ")
	challenge = strings.TrimPrefix(challenge, "digest ")
	for _, part := range splitDigestParams(challenge) {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.TrimSpace(kv[0])
		val := strings.Trim(strings.TrimSpace(kv[1]), `"`)
		out[key] = val
	}
	return out
}

// splitDigestParams splits on commas that are not inside quotes.
func splitDigestParams(s string) []string {
	var parts []string
	var cur strings.Builder
	inQuote := false
	for _, r := range s {
		switch r {
		case '"':
			inQuote = !inQuote
			cur.WriteRune(r)
		case ',':
			if inQuote {
				cur.WriteRune(r)
			} else {
				parts = append(parts, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		parts = append(parts, cur.String())
	}
	return parts
}

func md5hex(s string) string {
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "0000000000000000"[:n*2]
	}
	return hex.EncodeToString(b)
}
