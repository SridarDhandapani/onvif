package onvif

import (
	"bytes"
	"crypto/rand"
	"crypto/sha1"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// generatePasswordDigest creates a WS-Security UsernameToken password digest:
// Base64(SHA1(nonce + created + password)), with a cryptographically random
// nonce per the WS-Security spec. Returns the digest, the Base64 nonce, and the
// Created timestamp.
func generatePasswordDigest(password string) (digest, nonceB64, created string) {
	created = time.Now().UTC().Format("2006-01-02T15:04:05.000Z")

	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		// Extremely unlikely; fall back to a time-derived nonce.
		nonce = []byte(fmt.Sprintf("%d", time.Now().UnixNano()))
	}
	nonceB64 = base64.StdEncoding.EncodeToString(nonce)

	h := sha1.New()
	h.Write(nonce)
	h.Write([]byte(created))
	h.Write([]byte(password))
	digest = base64.StdEncoding.EncodeToString(h.Sum(nil))

	return digest, nonceB64, created
}

// sendSOAPRequest sends a SOAP request to an ONVIF device
func (c *Client) sendSOAPRequest(endpoint, action, body string) ([]byte, error) {
	digest, nonce, created := generatePasswordDigest(c.Password)

	authHeader := ""
	if c.Username != "" {
		authHeader = fmt.Sprintf(`
		<wsse:Security s:mustUnderstand="1" xmlns:wsse="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-secext-1.0.xsd" xmlns:wsu="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-utility-1.0.xsd">
			<wsse:UsernameToken>
				<wsse:Username>%s</wsse:Username>
				<wsse:Password Type="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-username-token-profile-1.0#PasswordDigest">%s</wsse:Password>
				<wsse:Nonce EncodingType="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-soap-message-security-1.0#Base64Binary">%s</wsse:Nonce>
				<wsu:Created>%s</wsu:Created>
			</wsse:UsernameToken>
		</wsse:Security>`, escapeXML(c.Username), digest, nonce, created)
	}

	soapRequest := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"
            xmlns:tds="http://www.onvif.org/ver10/device/wsdl"
            xmlns:trt="http://www.onvif.org/ver10/media/wsdl"
            xmlns:tt="http://www.onvif.org/ver10/schema"
            xmlns:timg="http://www.onvif.org/ver20/imaging/wsdl"
            xmlns:tr2="http://www.onvif.org/ver20/media/wsdl"
            xmlns:tptz="http://www.onvif.org/ver20/ptz/wsdl">
	<s:Header>%s</s:Header>
	<s:Body>%s</s:Body>
</s:Envelope>`, authHeader, body)

	req, err := http.NewRequest("POST", endpoint, bytes.NewBufferString(soapRequest))
	if err != nil {
		return nil, err
	}

	// SOAP 1.2 conveys the action as a Content-Type parameter. Keep the legacy
	// SOAPAction header too for devices that still look for it (SOAP 1.1 style).
	req.Header.Set("Content-Type", fmt.Sprintf("application/soap+xml; charset=utf-8; action=%q", action))
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
		// gSOAP-based devices (e.g. Reolink) return a SOAP fault with the real
		// reason in the body even on HTTP errors. Surface the fault Subcode and
		// Reason; fall back to a flattened body snippet if it isn't a fault.
		detail := faultDetail(respBody)
		if detail == "" {
			detail = strings.Join(strings.Fields(string(respBody)), " ")
			if len(detail) > 400 {
				detail = detail[:400]
			}
		}
		return respBody, fmt.Errorf("HTTP %d (%s): %s", resp.StatusCode, http.StatusText(resp.StatusCode), detail)
	}

	return respBody, nil
}

// faultDetail returns a human-readable reason (fault Subcode and/or Reason) for
// a SOAP fault body, or "" if the body is not a recognisable SOAP fault.
func faultDetail(resp []byte) string {
	if err := parseSOAPFault(resp); err != nil {
		return err.Error()
	}
	return ""
}

// getFirstAddress extracts the first address if multiple are provided
func getFirstAddress(address string) string {
	addresses := strings.Fields(address)
	if len(addresses) > 0 {
		return addresses[0]
	}
	return address
}
