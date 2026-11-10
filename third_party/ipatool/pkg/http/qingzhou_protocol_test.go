package http

import "testing"

func TestAppleDocumentProtocolEnvelope(t *testing.T) {
	var raw map[string]any
	err := DecodePropertyList([]byte(`<Document><Protocol><key>customerMessage</key><string>AMD-Action:SP</string></Protocol></Document>`), &raw)
	if err != nil || raw["customerMessage"] != "AMD-Action:SP" {
		t.Fatal("Apple protocol envelope was not decoded", err)
	}
}
