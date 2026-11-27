package commands

import (
	"testing"

	"github.com/ifc7/ifc/pkg/plugins/contract"
)

func TestSdkLanguage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in      string
		want    contract.SdkLanguage
		wantErr bool
	}{
		{"", contract.SdkLanguageGo, false},
		{"go", contract.SdkLanguageGo, false},
		{"Golang", contract.SdkLanguageGo, false},
		{"typescript", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()
			got, err := sdkLanguage(tt.in)
			if (err != nil) != tt.wantErr || got != tt.want {
				t.Fatalf("sdkLanguage(%q) = %q, %v", tt.in, got, err)
			}
		})
	}
}
