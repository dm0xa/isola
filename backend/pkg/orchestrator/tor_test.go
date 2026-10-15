package orchestrator

import (
	"reflect"
	"testing"
)

func TestTorSessionArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		enabled bool
		country string
		want    []string
		wantErr bool
	}{
		{name: "disabled yields no args", enabled: false, want: nil},
		{name: "enabled without a country", enabled: true, want: []string{"-e", "TOR_ENABLED=true"}},
		{name: "blank country means any exit", enabled: true, country: "   ", want: []string{"-e", "TOR_ENABLED=true"}},
		{name: "country is normalized and passed", enabled: true, country: "SE", want: []string{"-e", "TOR_ENABLED=true", "-e", "TOR_EXIT_COUNTRY=se"}},
		{name: "list is passed through", enabled: true, country: "se,no", want: []string{"-e", "TOR_ENABLED=true", "-e", "TOR_EXIT_COUNTRY=se,no"}},
		{name: "invalid country is an error", enabled: true, country: "swe", wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := torSessionArgs(tt.enabled, tt.country)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("torSessionArgs(%v, %q) = %v, want an error", tt.enabled, tt.country, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("torSessionArgs(%v, %q): %v", tt.enabled, tt.country, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("torSessionArgs(%v, %q) = %v, want %v", tt.enabled, tt.country, got, tt.want)
			}
		})
	}
}
