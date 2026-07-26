package project

import (
	"os"
	"testing"

	"github.com/ifc7/ifc/internal/client"
	"github.com/ifc7/ifc/internal/pkg/testutils"
)

func TestProject_Status(t *testing.T) {
	testutils.UseSandbox(t)

	content := []byte("openapi: 3.0.0\ninfo:\n  title: t\n  version: 0.1.0\npaths: {}\n")
	checksum := sha256Checksum(content)
	path := "api.yaml"
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}

	modifiedPath := "modified.yaml"
	if err := os.WriteFile(modifiedPath, append(content, []byte("# changed\n")...), 0o644); err != nil {
		t.Fatal(err)
	}

	for name, tc := range map[string]struct {
		config   Config
		manifest Manifest
		wantKind InterfaceStatusKind
	}{
		"clean": {
			config: Config{
				Own: []Owned{{Name: "api", Path: path}},
			},
			manifest: Manifest{
				Interfaces: map[string]*ManifestInterface{
					"api": {
						Interface: client.Interface{
							Name: "api",
							LatestRevision: &client.InterfaceRevision{
								Checksum: checksum,
							},
						},
					},
				},
			},
			wantKind: StatusClean,
		},
		"modified": {
			config: Config{
				Own: []Owned{{Name: "api", Path: modifiedPath}},
			},
			manifest: Manifest{
				Interfaces: map[string]*ManifestInterface{
					"api": {
						Interface: client.Interface{
							Name: "api",
							LatestRevision: &client.InterfaceRevision{
								Checksum: checksum,
							},
						},
					},
				},
			},
			wantKind: StatusModified,
		},
		"new": {
			config: Config{
				Own: []Owned{{Name: "api", Path: path}},
			},
			manifest: Manifest{Interfaces: map[string]*ManifestInterface{}},
			wantKind: StatusNew,
		},
		"missing": {
			config: Config{
				Own: []Owned{{Name: "api", Path: "does-not-exist.yaml"}},
			},
			manifest: Manifest{Interfaces: map[string]*ManifestInterface{}},
			wantKind: StatusMissing,
		},
		"clean via interface id ref": {
			config: Config{
				Own: []Owned{{Name: "api", Path: path, Ref: "interface_abc"}},
			},
			manifest: Manifest{
				Interfaces: map[string]*ManifestInterface{
					"interface_abc": {
						Interface: client.Interface{
							Id:   "interface_abc",
							Name: "api",
							LatestRevision: &client.InterfaceRevision{
								Checksum: checksum,
							},
						},
					},
				},
			},
			wantKind: StatusClean,
		},
		"clean via canonical path ref": {
			config: Config{
				Own: []Owned{{Name: "api", Path: path, Ref: "ifc7.dev/i/@user/api"}},
			},
			manifest: Manifest{
				Interfaces: map[string]*ManifestInterface{
					"interface_abc": {
						Interface: client.Interface{
							Id:           "interface_abc",
							Name:         "api",
							CanonicalUrl: "/i/@user/api",
							LatestRevision: &client.InterfaceRevision{
								Checksum: checksum,
							},
						},
					},
				},
			},
			wantKind: StatusClean,
		},
	} {
		t.Run(name, func(t *testing.T) {
			proj, err := projectWithMockClient(t, tc.config, tc.manifest, nil)
			if err != nil {
				t.Fatal(err)
			}
			statuses, err := proj.Status(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if len(statuses) != 1 {
				t.Fatalf("expected 1 status, got %d", len(statuses))
			}
			if statuses[0].Kind != tc.wantKind {
				t.Fatalf("expected kind %q, got %q (%s)", tc.wantKind, statuses[0].Kind, statuses[0].Detail)
			}
		})
	}
}

func TestFormatStatusReport_empty(t *testing.T) {
	got := FormatStatusReport(nil)
	if got != "No owned interfaces tracked in ifc.yaml.\n" {
		t.Fatalf("unexpected report: %q", got)
	}
}

func TestProject_Status_noOwned(t *testing.T) {
	proj, err := projectWithMockClient(t, Config{}, Manifest{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	statuses, err := proj.Status(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if statuses != nil {
		t.Fatalf("expected nil statuses, got %#v", statuses)
	}
}
