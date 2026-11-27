package runner

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ifc7/ifc/pkg/plugins/contract"
)

type slowLinter struct{ d time.Duration }

func (slowLinter) ID() string { return "slow-lint@test" }
func (s slowLinter) Lint(contract.LintInput) (contract.LintOutput, error) {
	time.Sleep(s.d)
	return contract.LintOutput{Score: 100}, nil
}

type slowComparator struct{ d time.Duration }

func (slowComparator) ID() string { return "slow-compare@test" }
func (s slowComparator) Compare(contract.CompareInput) (contract.CompareOutput, error) {
	time.Sleep(s.d)
	return contract.CompareOutput{}, nil
}

type slowGenerator struct{ d time.Duration }

func (slowGenerator) ID() string { return "slow-generate@test" }
func (s slowGenerator) Generate(contract.GenerationInput) (contract.GenerationOutput, error) {
	time.Sleep(s.d)
	return contract.GenerationOutput{}, nil
}

func slowRegistry(d time.Duration) *Registry {
	return &Registry{
		linters:     map[contract.InterfaceType]Linter{contract.InterfaceTypeOpenAPI: slowLinter{d}},
		comparators: map[contract.InterfaceType]Comparator{contract.InterfaceTypeOpenAPI: slowComparator{d}},
		generators:  map[generatorKey]Generator{{contract.InterfaceTypeOpenAPI, contract.SdkLanguageGo}: slowGenerator{d}},
	}
}

func TestRegistry_LintContext(t *testing.T) {
	t.Parallel()
	in := contract.LintInput{InterfaceType: contract.InterfaceTypeOpenAPI}
	t.Run("within budget", func(t *testing.T) {
		t.Parallel()
		out, id, err := slowRegistry(time.Millisecond).LintContext(context.Background(), in)
		if err != nil || out.Score != 100 || id != "slow-lint@test" {
			t.Fatalf("out=%+v id=%s err=%v", out, id, err)
		}
	})
	t.Run("over budget", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()
		_, id, err := slowRegistry(time.Second).LintContext(ctx, in)
		if !errors.Is(err, ErrBudgetExceeded) || id != "slow-lint@test" {
			t.Fatalf("id=%s err=%v", id, err)
		}
	})
	t.Run("cancelled before start", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, _, err := slowRegistry(0).LintContext(ctx, in)
		if !errors.Is(err, context.Canceled) || errors.Is(err, ErrBudgetExceeded) {
			t.Fatalf("err=%v", err)
		}
	})
}

func TestRegistry_CompareContext(t *testing.T) {
	t.Parallel()
	in := contract.CompareInput{InterfaceType: contract.InterfaceTypeOpenAPI}
	t.Run("within budget", func(t *testing.T) {
		t.Parallel()
		if _, _, err := slowRegistry(time.Millisecond).CompareContext(context.Background(), in); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("over budget", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()
		if _, _, err := slowRegistry(time.Second).CompareContext(ctx, in); !errors.Is(err, ErrBudgetExceeded) {
			t.Fatalf("err=%v", err)
		}
	})
}

func TestRegistry_GenerateContext(t *testing.T) {
	t.Parallel()
	in := contract.GenerationInput{
		InterfaceType: contract.InterfaceTypeOpenAPI,
		Target:        contract.GenerationTarget{Language: contract.SdkLanguageGo},
	}
	t.Run("within budget", func(t *testing.T) {
		t.Parallel()
		_, id, err := slowRegistry(time.Millisecond).GenerateContext(context.Background(), in)
		if err != nil {
			t.Fatal(err)
		}
		if id != "slow-generate@test" {
			t.Fatalf("id=%q", id)
		}
	})
	t.Run("over budget", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()
		_, id, err := slowRegistry(time.Second).GenerateContext(ctx, in)
		if !errors.Is(err, ErrBudgetExceeded) {
			t.Fatalf("err=%v", err)
		}
		if id != "slow-generate@test" {
			t.Fatalf("id=%q", id)
		}
	})
}
