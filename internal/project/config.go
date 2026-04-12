package project

import (
	"bytes"
	"cmp"
	"fmt"

	"golang.org/x/exp/slices"
	"gopkg.in/yaml.v3"

	"github.com/ifc7/ifc/internal/pkg/fileio"
)

var (
	ErrInvalidConfig = fmt.Errorf("invalid config")
	ErrRefExists     = fmt.Errorf("reference already exists")
	ErrRefNotFound   = fmt.Errorf("reference not found")
	ErrNameExists    = fmt.Errorf("name already exists")
	ErrPathExists    = fmt.Errorf("path already exists")
	ErrPathRequired  = fmt.Errorf("path is required for owned interfaces")
	ErrNameRequired  = fmt.Errorf("name is required for owned interfaces")
	ErrNameNotFound  = fmt.Errorf("name not found")
)

// Config represents a project configuration file
// This file holds references to interfaces that are tracked by the project.
type Config struct {
	Use []Used  `json:"use" yaml:"use"`
	Own []Owned `json:"own" yaml:"own"`
}

// Used holds a reference to a remote interface that is tracked by the project.
type Used struct {
	Ref string `json:"ref" yaml:"ref"`
}

// Owned holds a reference to a local interface that is managed by the project.
type Owned struct {
	Name string `json:"name" yaml:"name"`
	Ref  string `json:"ref" yaml:"ref"`
	Path string `json:"path" yaml:"path"`
}

// NewConfig creates a new empty Config struct
func NewConfig() *Config {
	return &Config{
		Use: make([]Used, 0),
		Own: make([]Owned, 0),
	}
}

// ReadConfig reads the project configuration file from disk
func ReadConfig(path string) (*Config, error) {
	b, err := fileio.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%w: error reading config file: %w", ErrInvalidConfig, err)
	}
	config := &Config{}
	decoder := yaml.NewDecoder(bytes.NewReader(b))
	decoder.KnownFields(true)
	err = decoder.Decode(config)
	if err != nil {
		return nil, fmt.Errorf("%w: error unmarshaling config: %w", ErrInvalidConfig, err)
	}
	if config.Use == nil {
		config.Use = make([]Used, 0)
	}
	if config.Own == nil {
		config.Own = make([]Owned, 0)
	}
	return config, nil
}

// Write writes the project configuration file to disk
func (c *Config) Write(path string) error {
	configFile, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("error marshaling config: %w", err)
	}
	err = fileio.WriteFile(configFile, path)
	if err != nil {
		return fmt.Errorf("error writing config file: %w", err)
	}
	return nil
}

// addUsedInterface adds a reference to a remote interface to the project configuration
func (c *Config) addUsedInterface(ref string) error {
	if exists, _ := c.refExists(ref); exists {
		return ErrRefExists
	}
	updatedUse := append(c.Use, Used{Ref: ref})
	// sort by reference
	slices.SortStableFunc(updatedUse, func(a, b Used) int {
		return cmp.Compare(a.Ref, b.Ref)
	})
	c.Use = updatedUse
	return nil
}

// rmUsedInterface removes a reference to a remote interface from the project configuration
func (c *Config) rmUsedInterface(ref string) error {
	if exists, owned := c.refExists(ref); !exists || owned {
		return ErrRefNotFound
	}
	var updatedUse []Used
	for i, u := range c.Use {
		if u.Ref != ref {
			updatedUse = slices.Delete(c.Use, i, i+1)
		}
	}
	if updatedUse == nil {
		updatedUse = make([]Used, 0)
	}
	c.Use = updatedUse
	return nil
}

// addOwnedInterface adds a reference to a local interface to the project configuration
func (c *Config) addOwnedInterface(name string, path string, ref string) error {
	if refExists, _ := c.refExists(ref); refExists {
		return ErrRefExists
	}
	if nameExists := c.nameExists(name); nameExists {
		return ErrNameExists
	}
	if pathExists := c.pathExists(path); pathExists {
		return ErrPathExists
	}
	if path == "" {
		return ErrPathRequired
	}
	if name == "" {
		return ErrNameRequired
	}
	updatedOwn := append(c.Own, Owned{Name: name, Path: path, Ref: ref})
	c.Own = updatedOwn
	// sort by name
	slices.SortStableFunc(updatedOwn, func(a, b Owned) int {
		return cmp.Compare(a.Name, b.Name)
	})
	c.Own = updatedOwn
	return nil
}

// rmOwnedInterface removes a reference to a local interface from the project configuration
func (c *Config) rmOwnedInterface(name string) error {
	if !c.nameExists(name) {
		return ErrNameNotFound
	}
	var updatedOwn []Owned
	for i, o := range c.Own {
		if o.Name != name {
			updatedOwn = slices.Delete(c.Own, i, i+1)
		}
	}
	if updatedOwn == nil {
		updatedOwn = make([]Owned, 0)
	}
	c.Own = updatedOwn
	return nil
}

// updateOwnedInterfacePath updates the path of a local interface in the project configuration
func (c *Config) updateOwnedInterfacePath(name string, path string) error {
	if !c.nameExists(name) {
		return ErrNameNotFound
	}
	if c.pathExists(path) {
		for _, o := range c.Own {
			if o.Name == name && o.Path == path {
				// handle no-op case
				return nil
			}
		}
		return ErrPathExists
	}
	for i, o := range c.Own {
		if o.Name == name {
			c.Own[i].Path = path
			return nil
		}
	}
	return nil
}

// updateOwnedInterfaceRef updates the ref of a local interface in the project configuration
func (c *Config) updateOwnedInterfaceRef(name string, ref string) error {
	if !c.nameExists(name) {
		return ErrNameNotFound
	}
	if refExists, _ := c.refExists(ref); refExists {
		for _, o := range c.Own {
			if o.Name == name && o.Ref == ref {
				// handle no-op case
				return nil
			}
		}
		return ErrRefExists
	}
	for i, o := range c.Own {
		if o.Name == name {
			c.Own[i].Ref = ref
			return nil
		}
	}
	return nil
}

// refExists checks if a reference already exists in the project configuration either in "use" or "own"
func (c *Config) refExists(ref string) (exists bool, owned bool) {
	if ref == "" {
		return false, false
	}
	for _, u := range c.Use {
		if u.Ref == ref {
			return true, false
		}
	}
	for _, o := range c.Own {
		if o.Ref == ref {
			return true, true
		}
	}
	return false, false
}

// nameExists checks if a name already exists in the project configuration
func (c *Config) nameExists(name string) bool {
	for _, o := range c.Own {
		if o.Name == name {
			return true
		}
	}
	return false
}

// pathExists checks if a path already exists in the project configuration
func (c *Config) pathExists(path string) bool {
	for _, o := range c.Own {
		if o.Path == path {
			return true
		}
	}
	return false
}
