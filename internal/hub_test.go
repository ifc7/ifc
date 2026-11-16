package internal

import "testing"

func TestAPIURL(t *testing.T) {
	for name, tc := range map[string]struct {
		env    string
		exp    string
		expErr bool
	}{
		"default":        {exp: "https://ifc7.dev/api/v0/"},
		"override":       {env: "https://staging.ifc7.dev", exp: "https://staging.ifc7.dev/api/v0/"},
		"trailing slash": {env: "http://localhost:8080/", exp: "http://localhost:8080/api/v0/"},
		"path prefix":    {env: "https://example.com/hub/", exp: "https://example.com/hub/api/v0/"},
		"no scheme":      {env: "ifc7.dev", expErr: true},
		"no host":        {env: "https://", expErr: true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv(EnvHubURL, tc.env)
			got, err := APIURL()
			if tc.expErr {
				if err == nil {
					t.Fatalf("expected error, got %s", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.exp {
				t.Fatalf("got %s, want %s", got, tc.exp)
			}
			hub, err := HubURLFromAPIURL(got)
			if err != nil {
				t.Fatal(err)
			}
			want, _ := HubURL()
			if hub.String() != want.String() {
				t.Fatalf("HubURLFromAPIURL(%s) = %s, want %s", got, hub, want)
			}
		})
	}
}
