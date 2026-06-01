package onvif

import (
	"strings"
	"testing"
)

func TestRotationMappingRoundTrip(t *testing.T) {
	for _, mode := range []RotationMode{RotationOff, Rotation90, Rotation180, Rotation270} {
		rm, deg := rotationToRotate(mode)
		got := rotateToRotation(rm, deg)
		if got != mode {
			t.Errorf("round-trip %v -> (%q,%d) -> %v", mode, rm, deg, got)
		}
	}

	if rm, deg := rotationToRotate(RotationOff); rm != "OFF" || deg != 0 {
		t.Errorf("RotationOff -> (%q,%d), want (OFF,0)", rm, deg)
	}
	if rm, deg := rotationToRotate(Rotation180); rm != "ON" || deg != 180 {
		t.Errorf("Rotation180 -> (%q,%d), want (ON,180)", rm, deg)
	}
	// Unknown degree falls back to Off.
	if got := rotateToRotation("ON", 45); got != RotationOff {
		t.Errorf("rotateToRotation(ON,45) = %v, want RotationOff", got)
	}
}

func TestParseRotationOptions(t *testing.T) {
	// Real i-PRO/Panasonic response: supports OFF and ON, degrees 0 and 180 only.
	resp := []byte(`<?xml version="1.0"?>
<env:Envelope xmlns:env="http://www.w3.org/2003/05/soap-envelope" xmlns:tt="http://www.onvif.org/ver10/schema">
 <env:Body><GetVideoSourceConfigurationOptionsResponse xmlns="http://www.onvif.org/ver10/media/wsdl">
  <trt:Options xmlns:trt="http://www.onvif.org/ver10/media/wsdl">
   <tt:Extension><tt:Rotate>
    <tt:Mode>OFF</tt:Mode><tt:Mode>ON</tt:Mode>
    <tt:DegreeList><tt:Items>0</tt:Items><tt:Items>180</tt:Items></tt:DegreeList>
   </tt:Rotate></tt:Extension>
  </trt:Options>
 </GetVideoSourceConfigurationOptionsResponse></env:Body></env:Envelope>`)

	opts := parseRotationOptions(resp)
	if !opts.Supported {
		t.Error("expected Supported=true (ON present)")
	}
	if len(opts.Degrees) != 2 || opts.Degrees[0] != 0 || opts.Degrees[1] != 180 {
		t.Errorf("Degrees = %v, want [0 180]", opts.Degrees)
	}
}

func TestBuildSetVideoSourceBody(t *testing.T) {
	var vsc videoSourceXML
	vsc.Token = "VSC0"
	vsc.Name = "VideoSource"
	vsc.UseCount = 2
	vsc.SourceToken = "VS0"
	vsc.Bounds.Width = 1920
	vsc.Bounds.Height = 1080
	vsc.Rotate.Mode = "ON"
	vsc.Rotate.Degree = 180

	body := buildSetVideoSourceBody(vsc)
	for _, want := range []string{
		`<trt:Configuration token="VSC0">`,
		"<tt:SourceToken xmlns:tt=\"http://www.onvif.org/ver10/schema\">VS0</tt:SourceToken>",
		`width="1920"`,
		`height="1080"`,
		"<tt:Rotate>",
		"<tt:Mode>ON</tt:Mode>",
		"<tt:Degree>180</tt:Degree>",
		"<trt:ForcePersistence>true</trt:ForcePersistence>",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("buildSetVideoSourceBody missing %q\n--- body ---\n%s", want, body)
		}
	}

	// Mode=OFF must not emit a Degree element.
	vsc.Rotate.Mode = "OFF"
	vsc.Rotate.Degree = 0
	off := buildSetVideoSourceBody(vsc)
	if strings.Contains(off, "<tt:Degree>") {
		t.Errorf("OFF rotation should not include Degree:\n%s", off)
	}
	if !strings.Contains(off, "<tt:Mode>OFF</tt:Mode>") {
		t.Errorf("OFF rotation should include Mode OFF:\n%s", off)
	}
}
