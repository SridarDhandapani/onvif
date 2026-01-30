package onvif

import (
	"bytes"
	"crypto/sha1"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// generatePasswordDigest creates WS-Security password digest
func generatePasswordDigest(username, password string) (string, string, string) {
	created := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	nonce := fmt.Sprintf("%d", time.Now().UnixNano())
	nonceBytes := []byte(nonce)
	nonceB64 := base64.StdEncoding.EncodeToString(nonceBytes)

	h := sha1.New()
	h.Write(nonceBytes)
	h.Write([]byte(created))
	h.Write([]byte(password))
	digest := base64.StdEncoding.EncodeToString(h.Sum(nil))

	return digest, nonceB64, created
}

// sendSOAPRequest sends a SOAP request to an ONVIF device
func (c *Client) sendSOAPRequest(endpoint, action, body string) ([]byte, error) {
	digest, nonce, created := generatePasswordDigest(c.Username, c.Password)

	authHeader := ""
	if c.Username != "" {
		authHeader = fmt.Sprintf(`
		<Security xmlns="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-secext-1.0.xsd">
			<UsernameToken>
				<Username>%s</Username>
				<Password Type="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-username-token-profile-1.0#PasswordDigest">%s</Password>
				<Nonce EncodingType="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-soap-message-security-1.0#Base64Binary">%s</Nonce>
				<Created xmlns="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-utility-1.0.xsd">%s</Created>
			</UsernameToken>
		</Security>`, c.Username, digest, nonce, created)
	}

	soapRequest := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"
            xmlns:tds="http://www.onvif.org/ver10/device/wsdl"
            xmlns:trt="http://www.onvif.org/ver10/media/wsdl"
            xmlns:tt="http://www.onvif.org/ver10/schema"
            xmlns:timg="http://www.onvif.org/ver20/imaging/wsdl"
            xmlns:tr2="http://www.onvif.org/ver20/media/wsdl">
	<s:Header>%s</s:Header>
	<s:Body>%s</s:Body>
</s:Envelope>`, authHeader, body)

	req, err := http.NewRequest("POST", endpoint, bytes.NewBufferString(soapRequest))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/soap+xml; charset=utf-8")
	req.Header.Set("SOAPAction", action)

	timeout := c.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}

	client := &http.Client{Timeout: timeout}
	if c.InsecureTLS {
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Check HTTP status before parsing body — some cameras return
	// error codes with an empty body instead of a SOAP fault
	if resp.StatusCode >= 400 {
		if len(respBody) == 0 {
			return nil, fmt.Errorf("HTTP %d with empty response", resp.StatusCode)
		}
		return respBody, fmt.Errorf("HTTP %d: %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}

	return respBody, nil
}

// getFirstAddress extracts the first address if multiple are provided
func getFirstAddress(address string) string {
	addresses := strings.Fields(address)
	if len(addresses) > 0 {
		return addresses[0]
	}
	return address
}