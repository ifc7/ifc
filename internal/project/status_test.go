package project

import (
	"os"
	"strings"
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
		wantSlug string
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
							Slug: "api-slug",
							LatestRevision: &client.InterfaceRevision{
								Checksum: checksum,
							},
						},
					},
				},
			},
			wantKind: StatusClean,
			wantSlug: "api-slug",
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
							Slug: "api",
							LatestRevision: &client.InterfaceRevision{
								Checksum: checksum,
							},
						},
					},
				},
			},
			wantKind: StatusClean,
			wantSlug: "api",
		},
		"clean via canonical path ref": {
			config: Config{
				Own: []Owned{{Name: "api", Path: path, Ref: "ifc7.dev/i/@user/api"}},
			},
			manifest: Manifest{
				Interfaces: map[string]*ManifestInterface{
					"/i/@user/api": {
						Interface: client.Interface{
							Id:           "interface_abc",
							Name:         "api",
							Slug:         "api",
							CanonicalUrl: "/i/@user/api",
							LatestRevision: &client.InterfaceRevision{
								Checksum: checksum,
							},
						},
					},
				},
			},
			wantKind: StatusClean,
			wantSlug: "api",
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
			if statuses[0].Slug != tc.wantSlug {
				t.Fatalf("expected slug %q, got %q", tc.wantSlug, statuses[0].Slug)
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

func TestFormatStatusReport_includesSlugColumn(t *testing.T) {
	got := FormatStatusReport([]InterfaceStatus{
		{Name: "Petstore API", Slug: "petstore", Path: "api.yaml", Kind: StatusClean},
		{Name: "new-api", Path: "new.yaml", Kind: StatusNew, Detail: "not in local manifest"},
	})
	if !strings.Contains(got, "slug") {
		t.Fatalf("expected slug header, got %q", got)
	}
	if !strings.Contains(got, "petstore") {
		t.Fatalf("expected slug value, got %q", got)
	}
	foundPlaceholder := false
	for _, line := range strings.Split(got, "\n") {
		if !strings.Contains(line, "new-api") {
			continue
		}
		fields := strings.Fields(line)
		// status, name, slug, path, ...
		if len(fields) >= 4 && fields[2] == "-" {
			foundPlaceholder = true
		}
	}
	if !foundPlaceholder {
		t.Fatalf("expected slug placeholder for new-api, got %q", got)
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
