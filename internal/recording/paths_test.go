package recording

import "testing"

func TestNormalizeSaveSubdir(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{input: "", want: "/default"},
		{input: "anchor-a", want: "/anchor-a"},
		{input: "/主播A", want: "/主播A"},
		{input: "../bad", wantErr: true},
	}

	for _, tt := range tests {
		got, err := NormalizeSaveSubdir(tt.input)
		if tt.wantErr {
			if err == nil {
				t.Fatalf("NormalizeSaveSubdir(%q) expected error", tt.input)
			}
			continue
		}
		if err != nil {
			t.Fatalf("NormalizeSaveSubdir(%q): %v", tt.input, err)
		}
		if got != tt.want {
			t.Fatalf("NormalizeSaveSubdir(%q)=%q want %q", tt.input, got, tt.want)
		}
	}
}
