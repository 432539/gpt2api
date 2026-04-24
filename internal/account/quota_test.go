package account

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestDecodeBlockedFeatures(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{
			name: "string array",
			raw:  `["image_gen","image_edit"]`,
			want: []string{"image_gen", "image_edit"},
		},
		{
			name: "array of objects",
			raw:  `[{"feature":"image_gen","reason":"turnstile_required"},{"feature":"image_edit","reason":"quota_exhausted"}]`,
			want: []string{"image_gen(turnstile_required)", "image_edit(quota_exhausted)"},
		},
		{
			name: "nested object map",
			raw:  `{"image_gen":{"reason":"turnstile_required"},"image_edit":{"blocked":true}}`,
			want: []string{"image_edit", "image_gen(turnstile_required)"},
		},
		{
			name: "null",
			raw:  `null`,
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := decodeBlockedFeatures(json.RawMessage(tt.raw))
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("decodeBlockedFeatures() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestShouldThrottleFromProbe(t *testing.T) {
	tests := []struct {
		name  string
		probe probeOutcome
		want  bool
	}{
		{
			name:  "remaining zero",
			probe: probeOutcome{remaining: 0},
			want:  true,
		},
		{
			name: "blocked image feature",
			probe: probeOutcome{
				remaining:       5,
				blockedFeatures: []string{"image_gen(turnstile_required)"},
			},
			want: true,
		},
		{
			name: "non image block only",
			probe: probeOutcome{
				remaining:       5,
				blockedFeatures: []string{"file_upload"},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldThrottleFromProbe(tt.probe); got != tt.want {
				t.Fatalf("shouldThrottleFromProbe() = %v, want %v", got, tt.want)
			}
		})
	}
}
