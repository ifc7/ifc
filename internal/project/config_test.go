package project

import (
	"path"
	"reflect"
	"testing"

	"github.com/ifc7/ifc/internal/pkg/testutils"
)

var (
	testConfigPath = path.Join(testutils.TestdataPath, "test_config.yaml")
)

func TestNewConfig(t *testing.T) {
	for name, _ := range map[string]struct {
	}{
		"baseline": {},
	} {
		t.Run(name, func(t *testing.T) {
			cfg := NewConfig()
			if cfg == nil {
				t.Fatal("expected config to be non-nil")
			}
			if cfg.Own == nil {
				t.Fatal("expected config.Own to be non-nil")
			}
			if len(cfg.Own) != 0 {
				t.Fatal("expected config.Own to be empty")
			}
			if cfg.Use == nil {
				t.Fatal("expected config.Use to be non-nil")
			}
			if len(cfg.Use) != 0 {
				t.Fatal("expected config.Use to be empty")
			}
		})
	}
}

func TestReadConfig(t *testing.T) {
	for name, tc := range map[string]struct {
		path   string
		expCfg *Config
		expErr error
	}{
		"baseline": {
			path:   testConfigPath,
			expCfg: NewConfig(),
		},
		"invalid config format": {
			path:   testEmptyManifestPath,
			expErr: ErrInvalidConfig,
		},
	} {
		t.Run(name, func(t *testing.T) {
			cfg, err := ReadConfig(tc.path)
			if endTest := testutils.CheckErr(t, err, tc.expErr); endTest == true {
				return
			}
			if cfg == nil {
				t.Fatal("expected config to be non-nil")
			}
			if !reflect.DeepEqual(*cfg, *tc.expCfg) {
				t.Fatalf("expected config to be %v, got %v", *tc.expCfg, *cfg)
			}
		})
	}
}

func TestConfig_Write(t *testing.T) {
	for name, tc := range map[string]struct {
		cfg    *Config
		expErr error
	}{
		"baseline": {
			cfg: NewConfig(),
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := tc.cfg.Write(path.Join(testutils.SandboxPath, "TestConfig_Write.yaml"))
			if endTest := testutils.CheckErr(t, err, tc.expErr); endTest == true {
				return
			}
		})
	}
}

func TestConfig_addUsedInterface(t *testing.T) {
	for name, tc := range map[string]struct {
		startCfg *Config
		addIfc   string
		expCfg   *Config
		expErr   error
	}{
		"baseline": {
			startCfg: &Config{
				Use: []Used{
					{
						Ref: "example.com/ifc/test",
					},
				},
				Own: []Owned{},
			},
			addIfc: "example.com/ifc/new",
			expCfg: &Config{
				Use: []Used{
					{
						Ref: "example.com/ifc/new",
					},
					{
						Ref: "example.com/ifc/test",
					},
				},
				Own: []Owned{},
			},
		},
		"ref already exists as used": {
			startCfg: &Config{
				Use: []Used{
					{
						Ref: "example.com/ifc/test",
					},
				},
				Own: []Owned{},
			},
			addIfc: "example.com/ifc/test",
			expErr: ErrRefExists,
		},
		"ref already exists as owned": {
			startCfg: &Config{
				Use: []Used{},
				Own: []Owned{
					{
						Name: "test",
						Path: "./schemas/test.json",
						Ref:  "example.com/ifc/test",
					},
				},
			},
			addIfc: "example.com/ifc/test",
			expErr: ErrRefExists,
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := tc.startCfg.addUsedInterface(tc.addIfc)
			if endTest := testutils.CheckErr(t, err, tc.expErr); endTest == true {
				return
			}
			if !reflect.DeepEqual(*tc.startCfg, *tc.expCfg) {
				t.Fatalf("expected config to be %v, got %v", *tc.expCfg, *tc.startCfg)
			}
		})
	}
}

func TestConfig_rmUsedInterface(t *testing.T) {
	for name, tc := range map[string]struct {
		startCfg *Config
		rmIfc    string
		expCfg   *Config
		expErr   error
	}{
		"baseline": {
			startCfg: &Config{
				Use: []Used{
					{
						Ref: "example.com/ifc/test",
					},
				},
				Own: []Owned{},
			},
			rmIfc:  "example.com/ifc/test",
			expCfg: NewConfig(),
		},
		"ref does not exist": {
			startCfg: NewConfig(),
			rmIfc:    "example.com/ifc/test",
			expErr:   ErrRefNotFound,
		},
		"ref is owned": {
			startCfg: &Config{
				Use: []Used{},
				Own: []Owned{
					{
						Name: "test",
						Path: "./schemas/test.json",
						Ref:  "example.com/ifc/test",
					},
				},
			},
			rmIfc:  "example.com/ifc/test",
			expErr: ErrRefNotFound,
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := tc.startCfg.rmUsedInterface(tc.rmIfc)
			if endTest := testutils.CheckErr(t, err, tc.expErr); endTest == true {
				return
			}
			if !reflect.DeepEqual(*tc.startCfg, *tc.expCfg) {
				t.Fatalf("expected config to be %v, got %v", *tc.expCfg, *tc.startCfg)
			}
		})
	}
}

func TestConfig_addOwnedInterface(t *testing.T) {
	for name, tc := range map[string]struct {
		startCfg *Config
		addIfc   Owned
		expCfg   *Config
		expErr   error
	}{
		"baseline": {
			startCfg: NewConfig(),
			addIfc: Owned{
				Name: "example",
				Path: "./schemas/example.json",
				Ref:  "example.com/ifc/example",
			},
			expCfg: &Config{
				Use: []Used{},
				Own: []Owned{
					{
						Name: "example",
						Path: "./schemas/example.json",
						Ref:  "example.com/ifc/example",
					},
				},
			},
		},
		"works without ref": {
			startCfg: &Config{
				Use: []Used{},
				Own: []Owned{
					{
						Name: "new",
						Path: "./schemas/new.json",
					},
				},
			},
			addIfc: Owned{
				Name: "example",
				Path: "./schemas/example.json",
			},
			expCfg: &Config{
				Use: []Used{},
				Own: []Owned{
					{
						Name: "example",
						Path: "./schemas/example.json",
					},
					{
						Name: "new",
						Path: "./schemas/new.json",
					},
				},
			},
		},
		"no name provided": {
			startCfg: NewConfig(),
			addIfc: Owned{
				Path: "./schemas/example.json",
			},
			expErr: ErrNameRequired,
		},
		"no path provided": {
			startCfg: NewConfig(),
			addIfc: Owned{
				Name: "example",
			},
			expErr: ErrPathRequired,
		},
		"name already exists": {
			startCfg: &Config{
				Use: []Used{},
				Own: []Owned{
					{
						Name: "example",
						Path: "./schemas/example.json",
						Ref:  "example.com/ifc/example",
					},
				},
			},
			addIfc: Owned{
				Name: "example",
				Path: "./schemas/new.json",
			},
			expErr: ErrNameExists,
		},
		"path already exists": {
			startCfg: &Config{
				Use: []Used{},
				Own: []Owned{
					{
						Name: "example",
						Path: "./schemas/example.json",
						Ref:  "example.com/ifc/example",
					},
				},
			},
			addIfc: Owned{
				Name: "new",
				Path: "./schemas/example.json",
			},
			expErr: ErrPathExists,
		},
		"ref already exists as used": {
			startCfg: &Config{
				Use: []Used{
					{
						Ref: "example.com/ifc/example",
					},
				},
				Own: []Owned{},
			},
			addIfc: Owned{
				Name: "example",
				Path: "./schemas/example.json",
				Ref:  "example.com/ifc/example",
			},
			expErr: ErrRefExists,
		},
		"ref already exists as owned": {
			startCfg: &Config{
				Use: []Used{},
				Own: []Owned{
					{
						Name: "new",
						Path: "./schemas/new.json",
						Ref:  "example.com/ifc/example",
					},
				},
			},
			addIfc: Owned{
				Name: "example",
				Path: "./schemas/example.json",
				Ref:  "example.com/ifc/example",
			},
			expErr: ErrRefExists,
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := tc.startCfg.addOwnedInterface(tc.addIfc.Name, tc.addIfc.Path, tc.addIfc.Ref)
			if endTest := testutils.CheckErr(t, err, tc.expErr); endTest == true {
				return
			}
			if !reflect.DeepEqual(*tc.startCfg, *tc.expCfg) {
				t.Fatalf("expected config to be %v, got %v", *tc.expCfg, *tc.startCfg)
			}
		})
	}
}

func TestConfig_rmOwnedInterface(t *testing.T) {
	for name, tc := range map[string]struct {
		startCfg  *Config
		rmIfcName string
		expCfg    *Config
		expErr    error
	}{
		"baseline": {
			startCfg: &Config{
				Use: []Used{},
				Own: []Owned{
					{
						Name: "example",
						Path: "./schemas/example.json",
					},
				},
			},
			rmIfcName: "example",
			expCfg:    NewConfig(),
		},
		"name does not exist": {
			startCfg: &Config{
				Use: []Used{},
				Own: []Owned{
					{
						Name: "example",
						Path: "./schemas/example.json",
					},
				},
			},
			rmIfcName: "new",
			expErr:    ErrNameNotFound,
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := tc.startCfg.rmOwnedInterface(tc.rmIfcName)
			if endTest := testutils.CheckErr(t, err, tc.expErr); endTest == true {
				return
			}
			if !reflect.DeepEqual(*tc.startCfg, *tc.expCfg) {
				t.Fatalf("expected config to be %v, got %v", *tc.expCfg, *tc.startCfg)
			}
		})
	}
}

func TestConfig_updateOwnedInterfacePath(t *testing.T) {
	for name, tc := range map[string]struct {
		startCfg      *Config
		updateIfcName string
		updateIfcPath string
		expCfg        *Config
		expErr        error
	}{
		"baseline": {
			startCfg: &Config{
				Use: []Used{},
				Own: []Owned{
					{
						Name: "example",
						Path: "./schemas/example.json",
					},
				},
			},
			updateIfcName: "example",
			updateIfcPath: "./schemas/new.json",
			expCfg: &Config{
				Use: []Used{},
				Own: []Owned{
					{
						Name: "example",
						Path: "./schemas/new.json",
					},
				},
			},
		},
		"name does not exist": {
			startCfg: &Config{
				Use: []Used{},
				Own: []Owned{
					{
						Name: "example",
						Path: "./schemas/example.json",
					},
				},
			},
			updateIfcName: "new",
			updateIfcPath: "./schemas/new.json",
			expErr:        ErrNameNotFound,
		},
		"path exists": {
			startCfg: &Config{
				Use: []Used{},
				Own: []Owned{
					{
						Name: "new",
						Path: "./schemas/new.json",
					},
					{
						Name: "example",
						Path: "./schemas/example.json",
					},
				},
			},
			updateIfcName: "example",
			updateIfcPath: "./schemas/new.json",
			expErr:        ErrPathExists,
		},
		"no-op": {
			startCfg: &Config{
				Use: []Used{},
				Own: []Owned{
					{
						Name: "example",
						Path: "./schemas/example.json",
					},
				},
			},
			updateIfcName: "example",
			updateIfcPath: "./schemas/example.json",
			expCfg: &Config{
				Use: []Used{},
				Own: []Owned{
					{
						Name: "example",
						Path: "./schemas/example.json",
					},
				},
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := tc.startCfg.updateOwnedInterfacePath(tc.updateIfcName, tc.updateIfcPath)
			if endTest := testutils.CheckErr(t, err, tc.expErr); endTest == true {
				return
			}
			if !reflect.DeepEqual(*tc.startCfg, *tc.expCfg) {
				t.Fatalf("expected config to be %v, got %v", *tc.expCfg, *tc.startCfg)
			}
		})
	}
}

func TestConfig_updateOwnedInterfaceRef(t *testing.T) {
	for name, tc := range map[string]struct {
		startCfg      *Config
		updateIfcName string
		updateIfcRef  string
		expCfg        *Config
		expErr        error
	}{
		"baseline": {
			startCfg: &Config{
				Use: []Used{},
				Own: []Owned{
					{
						Name: "example",
						Path: "./schemas/example.json",
					},
				},
			},
			updateIfcName: "example",
			updateIfcRef:  "example.com/ifc/example",
			expCfg: &Config{
				Use: []Used{},
				Own: []Owned{
					{
						Name: "example",
						Path: "./schemas/example.json",
						Ref:  "example.com/ifc/example",
					},
				},
			},
		},
		"name does not exist": {
			startCfg: &Config{
				Use: []Used{},
				Own: []Owned{
					{
						Name: "example",
						Path: "./schemas/example.json",
					},
				},
			},
			updateIfcName: "new",
			updateIfcRef:  "example.com/ifc/example",
			expErr:        ErrNameNotFound,
		},
		"ref exists as owned": {
			startCfg: &Config{
				Use: []Used{},
				Own: []Owned{
					{
						Name: "new",
						Path: "./schemas/new.json",
						Ref:  "example.com/ifc/new",
					},
					{
						Name: "example",
						Path: "./schemas/example.json",
						Ref:  "example.com/ifc/example",
					},
				},
			},
			updateIfcName: "example",
			updateIfcRef:  "example.com/ifc/new",
			expErr:        ErrRefExists,
		},
		"ref exists as used": {
			startCfg: &Config{
				Use: []Used{
					{
						Ref: "example.com/ifc/new",
					},
				},
				Own: []Owned{
					{
						Name: "example",
						Path: "./schemas/example.json",
						Ref:  "example.com/ifc/example",
					},
				},
			},
			updateIfcName: "example",
			updateIfcRef:  "example.com/ifc/new",
			expErr:        ErrRefExists,
		},
		"no-op": {
			startCfg: &Config{
				Use: []Used{},
				Own: []Owned{
					{
						Name: "example",
						Path: "./schemas/example.json",
						Ref:  "example.com/ifc/example",
					},
				},
			},
			updateIfcName: "example",
			updateIfcRef:  "example.com/ifc/example",
			expCfg: &Config{
				Use: []Used{},
				Own: []Owned{
					{
						Name: "example",
						Path: "./schemas/example.json",
						Ref:  "example.com/ifc/example",
					},
				},
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := tc.startCfg.updateOwnedInterfaceRef(tc.updateIfcName, tc.updateIfcRef)
			if endTest := testutils.CheckErr(t, err, tc.expErr); endTest == true {
				return
			}
			if !reflect.DeepEqual(*tc.startCfg, *tc.expCfg) {
				t.Fatalf("expected config to be %v, got %v", *tc.expCfg, *tc.startCfg)
			}
		})
	}
}
