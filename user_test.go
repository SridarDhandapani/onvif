package onvif

import "testing"

func TestExtractBetweenTags(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		localName string
		want      string
	}{
		{
			name:      "namespaced close tag (XAddr) does not leak prefix",
			input:     `<tt:XAddr>http://192.168.50.109/onvif</tt:XAddr>`,
			localName: "XAddr",
			want:      "http://192.168.50.109/onvif",
		},
		{
			name:      "unprefixed tag",
			input:     `<Uri>rtsp://cam/stream</Uri>`,
			localName: "Uri",
			want:      "rtsp://cam/stream",
		},
		{
			name:      "tag with attributes",
			input:     `<tt:Username Type="x">admin</tt:Username>`,
			localName: "Username",
			want:      "admin",
		},
		{
			name:      "value surrounded by other elements",
			input:     `<tt:Imaging><tt:XAddr>http://h/onvif</tt:XAddr></tt:Imaging>`,
			localName: "XAddr",
			want:      "http://h/onvif",
		},
		{
			name:      "missing close tag returns empty",
			input:     `<tt:XAddr>http://h/onvif`,
			localName: "XAddr",
			want:      "",
		},
		{
			name:      "missing element returns empty",
			input:     `<tt:Media></tt:Media>`,
			localName: "XAddr",
			want:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractBetweenTags(tt.input, tt.localName); got != tt.want {
				t.Errorf("extractBetweenTags(%q, %q) = %q, want %q", tt.input, tt.localName, got, tt.want)
			}
		})
	}
}
