package textutil

import "testing"

func TestDecodeJSONObjectRepairsSpacedEscapes(t *testing.T) {
	var out struct {
		Story string `json:"story"`
	}
	if err := DecodeJSONObject(`{"story":"第一行\ n第二行"}`, &out); err != nil {
		t.Fatalf("decode repaired json: %v", err)
	}
	if out.Story != "第一行\n第二行" {
		t.Fatalf("unexpected story: %q", out.Story)
	}
}
