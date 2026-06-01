# ONVIF compliance status

This library was hardened for ONVIF compliance while unifying multi-vendor
camera setup (Panasonic, Reolink, Honeywell). This file records what was done
and what is intentionally deferred.

## Done

- **Structured XML everywhere.** All response parsing uses `encoding/xml`
  structs (namespace-agnostic by local name). The ad-hoc string scanners
  (`extractBetweenTags`, `findTagOpen/Close`, `extractTextElement`,
  `extractServiceXAddr`, `containsSOAPFault`, and the hand-rolled
  `strings.Index("<tds:…>")` blocks) were removed. These caused real bugs
  (a trailing `</tt` on the Imaging `XAddr`, and missing OSDs because the
  response element is `OSDs`, not `OSD`).
- **Structured SOAP fault parsing.** `parseSOAPFault` unmarshals the fault and
  reports the Subcode + Reason (SOAP 1.2) or faultstring (SOAP 1.1). All call
  sites use it (replacing `bytes.Contains("SOAP-ENV:Fault")`, which missed
  other prefixes). HTTP >= 400 bodies surface the same detail.
- **Service discovery.** `discoverServices` resolves Media / Media2 / Imaging
  URLs from the device's own advertisements (`GetCapabilities`, then
  `GetServices`), instead of rewriting the device-service address. This is what
  lets OSD/Imaging work on cameras (e.g. Reolink) that serve ONVIF on a
  non-default port and per-service paths. Heuristic rewrites remain only as a
  last-resort fallback.
- **Encoder writes.** `SetVideoEncoderConfiguration` is a read-modify-write of
  the real `VideoEncoderConfiguration` (by token), not a fabricated one.
- **SetSystemDateAndTime** omits the optional `TimeZone` (clock sync only;
  avoids `ter:InvalidArgVal` on strict devices).
- **Discovery** no longer panics on a ProbeMatch with empty `XAddrs`.

## Deferred (TODO — re-test on all models when changing these)

These touch the auth/transport layer, so they re-affect **every** call on
**every** device. They were deferred deliberately because the three target
cameras currently authenticate with the existing code; change them only with a
full end-to-end re-test.

### D. WS-Security UsernameToken (`soap.go`)

Current `generatePasswordDigest` / `sendSOAPRequest`:

- **Nonce is not random** — it is `time.Now().UnixNano()` formatted as a decimal
  string. WS-Security requires a cryptographically random nonce. Fix: generate
  ~16 bytes via `crypto/rand`, use the raw bytes in the digest
  (`SHA1(nonce + created + password)`) and base64 of the raw bytes in
  `<Nonce>`.
- **Header is not the canonical form** — it uses a default `xmlns` with no
  `wsse:`/`wsu:` prefixes and no `s:mustUnderstand="1"`. Fix: emit
  `<wsse:Security s:mustUnderstand="1">` with `wsse:UsernameToken`,
  `wsse:Password Type="…#PasswordDigest"`, `wsse:Nonce`, and `wsu:Created`
  in the WS-Security utility namespace.

### E. SOAP 1.2 action binding (`soap.go`)

The envelope is SOAP 1.2 (`application/soap+xml`), but the action is sent only
as a separate `SOAPAction` HTTP header (SOAP 1.1 style). SOAP 1.2 conveys the
action as a Content-Type parameter:

```
Content-Type: application/soap+xml; charset=utf-8; action="<action-uri>"
```

Fix: add the `action` parameter to the Content-Type. Keeping the `SOAPAction`
header as well is harmless and maximises device compatibility.

## Minor / optional

- `GetCapabilities` detects PTZ/Analytics support with `strings.Contains` on the
  raw response. Cosmetic flags only; could be structured but low priority.
- `discovery.go` `parseScopes`/`parseProfiles` match scope/type URIs with string
  containment — acceptable, as these are URIs by definition.
