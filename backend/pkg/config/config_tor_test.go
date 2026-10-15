package config

import (
	"testing"
)

func TestNormalizeTorCountry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{name: "empty stays empty", in: "", want: ""},
		{name: "whitespace only stays empty", in: "   ", want: ""},
		{name: "single lowercase", in: "se", want: "se"},
		{name: "uppercase is lowercased", in: "SE", want: "se"},
		{name: "surrounding whitespace is trimmed", in: "  se  ", want: "se"},
		{name: "list is preserved in order", in: "se,no", want: "se,no"},
		{name: "duplicates collapse", in: "se,SE,se", want: "se"},
		{name: "blank entries are ignored", in: "se,,no", want: "se,no"},
		{name: "only separators means empty", in: ",,", want: ""},
		{name: "three letters rejected", in: "swe", wantErr: true},
		{name: "digits rejected", in: "s1", wantErr: true},
		{name: "one letter rejected", in: "s", wantErr: true},
		{name: "ten codes allowed", in: "aa,ab,ac,ad,ae,af,ag,ah,ai,aj", want: "aa,ab,ac,ad,ae,af,ag,ah,ai,aj"},
		{name: "too many codes rejected", in: "aa,ab,ac,ad,ae,af,ag,ah,ai,aj,ak", wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := NormalizeTorCountry(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("NormalizeTorCountry(%q) = %q, want an error", tt.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeTorCountry(%q): %v", tt.in, err)
			}
			if got != tt.want {
				t.Fatalf("NormalizeTorCountry(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
