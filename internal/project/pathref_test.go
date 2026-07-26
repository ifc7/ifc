package project

import "testing"

func TestConfigRefFromCanonicalURL(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{in: "/i/acme/payments", want: "ifc7.dev/i/acme/payments"},
		{in: "/i/@shaun/my-api", want: "ifc7.dev/i/@shaun/my-api"},
		{in: "i/acme/payments", want: "ifc7.dev/i/acme/payments"},
		{in: "dev.ifc7.dev/i/acme/payments", want: "dev.ifc7.dev/i/acme/payments"},
		{in: "https://dev.ifc7.dev/i/acme/payments", want: "dev.ifc7.dev/i/acme/payments"},
		{in: "https://ifc7.dev/i/@shaun/my-api", want: "ifc7.dev/i/@shaun/my-api"},
		{in: "", wantErr: true},
		{in: "/api/v0/interfaces/x", wantErr: true},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			got, err := configRefFromCanonicalURL(c.in)
			if c.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Fatalf("got %q want %q", got, c.want)
			}
		})
	}
}

func TestParsePathRef(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in        string
		wantOK    bool
		wantCanon string
	}{
		{in: "ifc7.dev/i/acme/payments", wantOK: true, wantCanon: "ifc7.dev/i/acme/payments"},
		{in: "ifc7.dev/i/@shaun/my-api/v1.2.3", wantOK: true, wantCanon: "ifc7.dev/i/@shaun/my-api/v1.2.3"},
		{in: "https://staging.ifc7.dev/i/acme/api", wantOK: true, wantCanon: "staging.ifc7.dev/i/acme/api"},
		{in: "ifc7.dev/api/v0/i/o/interface7/rest-api", wantOK: false},
		{in: "invalid", wantOK: false},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			got, ok := parsePathRef(c.in)
			if ok != c.wantOK {
				t.Fatalf("ok=%v want %v", ok, c.wantOK)
			}
			if !c.wantOK {
				return
			}
			if got.canonical() != c.wantCanon {
				t.Fatalf("got %s want %s", got.canonical(), c.wantCanon)
			}
		})
	}
}
