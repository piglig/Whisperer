package fogharbor

import (
	"context"

	"github.com/zhuzhenwu/whisperer/internal/authoring"
)

type Adapter struct{}

func init() {
	authoring.Register(Adapter{})
}

func (Adapter) ID() string {
	return "fog_harbor"
}

func (Adapter) DisplayName() string {
	return "Fog Harbor"
}

func (Adapter) ValidatePath(path string) (string, error) {
	p, err := validatePath(path)
	return string(p), err
}

func (Adapter) RunPlaytest(ctx context.Context, opts authoring.PlaytestOptions) (authoring.PlaytestReport, error) {
	return runPlaytest(ctx, opts)
}

func (Adapter) RunAllPlaytests(ctx context.Context) ([]authoring.PlaytestReport, error) {
	return runAllPlaytests(ctx)
}

func (Adapter) Verify(ctx context.Context) (authoring.VerifyReport, error) {
	return authoring.VerifyBundledScenario(ctx, "fog_harbor", runAllPlaytests, customGates)
}
