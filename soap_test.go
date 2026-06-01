package onvif

import "testing"

func TestFaultDetail(t *testing.T) {
	fault := `<?xml version="1.0" encoding="UTF-8"?>
<SOAP-ENV:Envelope xmlns:SOAP-ENV="http://www.w3.org/2003/05/soap-envelope" xmlns:ter="http://www.onvif.org/ver10/error">
  <SOAP-ENV:Body>
    <SOAP-ENV:Fault>
      <SOAP-ENV:Code>
        <SOAP-ENV:Value>SOAP-ENV:Sender</SOAP-ENV:Value>
        <SOAP-ENV:Subcode>
          <SOAP-ENV:Value>ter:InvalidArgVal</SOAP-ENV:Value>
        </SOAP-ENV:Subcode>
      </SOAP-ENV:Code>
      <SOAP-ENV:Reason>
        <SOAP-ENV:Text xml:lang="en">The requested time is invalid</SOAP-ENV:Text>
      </SOAP-ENV:Reason>
    </SOAP-ENV:Fault>
  </SOAP-ENV:Body>
</SOAP-ENV:Envelope>`

	got := faultDetail([]byte(fault))
	want := "ter:InvalidArgVal: The requested time is invalid"
	if got != want {
		t.Errorf("faultDetail() = %q, want %q", got, want)
	}

	if d := faultDetail([]byte(`<ok/>`)); d != "" {
		t.Errorf("faultDetail(non-fault) = %q, want empty", d)
	}
}
