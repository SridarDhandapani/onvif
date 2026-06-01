package onvif

import (
	"strings"
	"testing"
)

func TestBuildPTZVectorXML(t *testing.T) {
	tests := []struct {
		name            string
		elem            string
		v               PTZVector
		panTilt, zoom   bool
		wantContains    []string
		wantNotContains []string
	}{
		{
			name: "velocity pan+tilt only",
			elem: "tptz:Velocity", v: PTZVector{Pan: 0.5, Tilt: -0.25}, panTilt: true, zoom: false,
			wantContains:    []string{"<tptz:Velocity>", `x="0.5"`, `y="-0.25"`, "</tptz:Velocity>", "tt:PanTilt"},
			wantNotContains: []string{"tt:Zoom"},
		},
		{
			name: "translation zoom only",
			elem: "tptz:Translation", v: PTZVector{Zoom: 0.1}, panTilt: false, zoom: true,
			wantContains:    []string{"<tptz:Translation>", `<tt:Zoom xmlns:tt="http://www.onvif.org/ver10/schema" x="0.1"/>`, "</tptz:Translation>"},
			wantNotContains: []string{"tt:PanTilt"},
		},
		{
			name: "position both",
			elem: "tptz:Position", v: PTZVector{Pan: 1, Tilt: 0, Zoom: 0.5}, panTilt: true, zoom: true,
			wantContains:    []string{"<tptz:Position>", "tt:PanTilt", "tt:Zoom"},
			wantNotContains: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildPTZVectorXML(tt.elem, tt.v, tt.panTilt, tt.zoom)
			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("buildPTZVectorXML() = %q, want contains %q", got, want)
				}
			}
			for _, no := range tt.wantNotContains {
				if strings.Contains(got, no) {
					t.Errorf("buildPTZVectorXML() = %q, want NOT contains %q", got, no)
				}
			}
		})
	}
}
