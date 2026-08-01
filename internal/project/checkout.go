package project

import (
	"bytes"
	"context"
	"fmt"

	"github.com/ifc7/ifc/internal/pkg/fileio"
)

// CheckoutParams holds parameters for Checkout.
type CheckoutParams struct {
	// Targets are owned interface names or slugs. Empty means all owned interfaces.
	Targets []string
	// Force overwrites local files that differ from the manifest latest revision.
	Force bool
}

// Checkout writes owned interface working-tree files from the latest revision in
// the local manifest. It does not contact the remote hub.
//
// Missing files are created. Files that already match the manifest are left
// unchanged. Files with local modifications are skipped unless Force is set.
func (p *Project) Checkout(ctx context.Context, params CheckoutParams) ([]string, error) {
	_ = ctx
	owned, err := p.resolveCheckoutTargets(params.Targets)
	if err != nil {
		return nil, err
	}
	var messages []string
	for _, own := range owned {
		msg, err := p.checkoutOwned(own, params.Force)
		if err != nil {
			return messages, err
		}
		if msg != "" {
			messages = append(messages, msg)
		}
	}
	return messages, nil
}

func (p *Project) resolveCheckoutTargets(targets []string) ([]Owned, error) {
	if len(targets) == 0 {
		if len(p.config.Own) == 0 {
			return nil, fmt.Errorf("no owned interfaces tracked in ifc.yaml")
		}
		return append([]Owned(nil), p.config.Own...), nil
	}
	owned := make([]Owned, 0, len(targets))
	for _, target := range targets {
		own, err := p.ownedByNameOrSlug(target)
		if err != nil {
			return nil, err
		}
		owned = append(owned, own)
	}
	return owned, nil
}

func (p *Project) checkoutOwned(own Owned, force bool) (string, error) {
	if own.Path == "" {
		return "", fmt.Errorf("owned interface %q has empty path", own.Name)
	}
	manifestBytes, err := p.manifestSpecification(own)
	if err != nil {
		return "", err
	}
	if fileio.FileExists(own.Path) {
		current, err := fileio.ReadFile(own.Path)
		if err != nil {
			return "", fmt.Errorf("error reading %s: %w", own.Path, err)
		}
		if bytes.Equal(current, manifestBytes) {
			return fmt.Sprintf("Interface %q is already up to date.", own.Name), nil
		}
		if !force {
			return fmt.Sprintf("Skipped %q: local modifications; run 'ifc diff %s' or use --force.", own.Name, own.Name), nil
		}
	}
	if err := fileio.WriteFile(manifestBytes, own.Path); err != nil {
		return "", fmt.Errorf("error writing %s: %w", own.Path, err)
	}
	return fmt.Sprintf("Updated %q (%s).", own.Name, own.Path), nil
}
