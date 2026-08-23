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

func TestDecodeJSONObjectCompletesTruncatedJSON(t *testing.T) {
	cases := []struct {
		name string
		text string
		want map[string]any
	}{
		{
			// Real glm-5.3 sample: response cut right before the final '}'.
			name: "missing outer brace from real glm output",
			text: `{"action":"tool_call","tool":"code.search","args":{"query":"func hasFinishEvidence","reason":"需要确认判定逻辑","summary":"","question":""}`,
			want: map[string]any{"action": "tool_call", "tool": "code.search"},
		},
		{
			name: "missing array and object closers",
			text: `{"items":[{"id":1},{"id":2}`,
			want: map[string]any{},
		},
		{
			name: "dangling string value",
			text: `{"summary":"写完测试文件`,
			want: map[string]any{"summary": "写完测试文件"},
		},
		{
			name: "trailing comma before truncation",
			text: `{"a":1,`,
			want: map[string]any{"a": float64(1)},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out map[string]any
			if err := DecodeJSONObject(tc.text, &out); err != nil {
				t.Fatalf("decode truncated json: %v", err)
			}
			for key, want := range tc.want {
				if out[key] != want {
					t.Errorf("field %q = %#v, want %#v", key, out[key], want)
				}
			}
		})
	}
}

func TestDecodeJSONObjectKeepsValidJSONUntouched(t *testing.T) {
	var out map[string]any
	if err := DecodeJSONObject(`{"a":{"b":[1,2]}}`, &out); err != nil {
		t.Fatalf("decode valid json: %v", err)
	}
	// Irreparably invalid input must still return an error.
	if err := DecodeJSONObject(`not json at all`, &out); err == nil {
		t.Fatal("expected error for non-json input")
	}
}

func TestCompleteTruncatedJSONReturnsInputWhenBalanced(t *testing.T) {
	if got := completeTruncatedJSON(`{"a":[1]}`); got != `{"a":[1]}` {
		t.Fatalf("balanced input should be unchanged, got %q", got)
	}
}
