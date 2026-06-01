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
- **WS-Security UsernameToken** uses a cryptographically random nonce
  (`crypto/rand`) and the canonical header form: `<wsse:Security
  s:mustUnderstand="1">` with `wsse:`/`wsu:` prefixes and `wsu:Created`.
- **SOAP 1.2 action** is sent as the `action` parameter of the
  `Content-Type` header; the legacy `SOAPAction` header is kept for
  SOAP 1.1-style devices.

## Minor / optional

- `GetCapabilities` detects PTZ/Analytics support with `strings.Contains` on the
  raw response. Cosmetic flags only; could be structured but low priority.
- `discovery.go` `parseScopes`/`parseProfiles` match scope/type URIs with string
  containment — acceptable, as these are URIs by definition.
