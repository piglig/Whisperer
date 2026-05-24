package fogharbor

import (
	"context"
	"fmt"

	"github.com/zhuzhenwu/whisperer/internal/authoring"
	"github.com/zhuzhenwu/whisperer/internal/scenario"
	"github.com/zhuzhenwu/whisperer/internal/store"
)

type playtestPath string

const (
	pathMainline  playtestPath = "mainline"
	pathFlee      playtestPath = "flee"
	pathDismissed playtestPath = "dismissed"
)

func runPlaytest(ctx context.Context, opts authoring.PlaytestOptions) (authoring.PlaytestReport, error) {
	path, err := validatePath(opts.Path)
	if err != nil {
		return authoring.PlaytestReport{}, err
	}
	base, err := scenario.LoadBundled("fog_harbor")
	if err != nil {
		return authoring.PlaytestReport{}, err
	}
	scn, variantID, err := authoring.SelectVariant(base, opts.VariantID)
	if err != nil {
		return authoring.PlaytestReport{}, err
	}
	env, err := authoring.NewEnv(ctx, scn, variantID)
	if err != nil {
		return authoring.PlaytestReport{}, err
	}
	defer env.Close()

	switch path {
	case pathMainline:
		err = runMainline(ctx, env)
	case pathFlee:
		err = runFlee(ctx, env)
	case pathDismissed:
		err = runDismissed(ctx, env)
	default:
		err = fmt.Errorf("unknown fog harbor playtest path %q", path)
	}
	if err != nil {
		return authoring.PlaytestReport{}, err
	}
	return env.Report(ctx, string(path), expectedEndings(path))
}

func runAllPlaytests(ctx context.Context) ([]authoring.PlaytestReport, error) {
	base, err := scenario.LoadBundled("fog_harbor")
	if err != nil {
		return nil, err
	}
	variants := authoring.VariantIDs(base)
	reports := []authoring.PlaytestReport{}
	for _, variantID := range variants {
		report, err := runPlaytest(ctx, authoring.PlaytestOptions{VariantID: variantID, Path: string(pathMainline)})
		if err != nil {
			return nil, err
		}
		reports = append(reports, report)
	}
	for _, path := range []playtestPath{pathFlee, pathDismissed} {
		report, err := runPlaytest(ctx, authoring.PlaytestOptions{VariantID: variants[0], Path: string(path)})
		if err != nil {
			return nil, err
		}
		reports = append(reports, report)
	}
	return reports, nil
}

func runMainline(ctx context.Context, env *authoring.Env) error {
	steps := []func(context.Context, *authoring.Env) error{
		func(ctx context.Context, e *authoring.Env) error {
			return e.Find(ctx, 1, "harbor", "blood_letter", "tide_chart")
		},
		func(ctx context.Context, e *authoring.Env) error { return e.RelateAt(ctx, 2, "station", "helena", 20) },
		func(ctx context.Context, e *authoring.Env) error {
			return e.RelateAt(ctx, 4, "church", "father_calvin", 20)
		},
		func(ctx context.Context, e *authoring.Env) error {
			return e.Visit(ctx, 8, "lighthouse", store.TimeNight)
		},
		func(ctx context.Context, e *authoring.Env) error { return e.Visit(ctx, 10, "pub", store.TimeNight) },
		func(ctx context.Context, e *authoring.Env) error {
			return e.Visit(ctx, 14, "lighthouse", store.TimeNight)
		},
		func(ctx context.Context, e *authoring.Env) error {
			return e.Find(ctx, 16, "reef_cave", "reef_carvings", "sacrifice_chamber")
		},
		func(ctx context.Context, e *authoring.Env) error { return confrontCulprit(ctx, e, 21) },
	}
	for _, step := range steps {
		if err := step(ctx, env); err != nil {
			return err
		}
	}
	return nil
}

func runFlee(ctx context.Context, env *authoring.Env) error {
	steps := []func(context.Context, *authoring.Env) error{
		func(ctx context.Context, e *authoring.Env) error {
			return e.Find(ctx, 1, "harbor", "blood_letter", "tide_chart")
		},
		func(ctx context.Context, e *authoring.Env) error {
			return e.RelateAt(ctx, 4, "church", "father_calvin", 20)
		},
		func(ctx context.Context, e *authoring.Env) error { return e.Visit(ctx, 7, "pub", store.TimeNight) },
		func(ctx context.Context, e *authoring.Env) error {
			return e.Visit(ctx, 11, "harbor", store.TimeMorning)
		},
	}
	for _, step := range steps {
		if err := step(ctx, env); err != nil {
			return err
		}
	}
	return nil
}

func runDismissed(ctx context.Context, env *authoring.Env) error {
	for turn, delta := range []int{-5, -5, -5, -5} {
		if err := env.RelateAt(ctx, turn+1, "station", "rourke", delta); err != nil {
			return err
		}
	}
	return env.Visit(ctx, 8, "station", store.TimeMorning)
}

func confrontCulprit(ctx context.Context, env *authoring.Env, turn int) error {
	switch env.VariantID() {
	case "calvin_directs":
		return env.Visit(ctx, turn, "church", store.TimeNight)
	case "rourke_runs":
		return env.Find(ctx, turn, "station", "rourke_bribe")
	default:
		return env.Visit(ctx, turn, "pub", store.TimeNight)
	}
}

func expectedEndings(path playtestPath) []string {
	switch path {
	case pathMainline:
		return []string{"pact_broken", "solved"}
	case pathFlee:
		return []string{"flee_with_truth"}
	case pathDismissed:
		return []string{"dismissed"}
	default:
		return nil
	}
}

func validatePath(path string) (playtestPath, error) {
	if path == "" {
		path = string(pathMainline)
	}
	parsed, err := authoring.ValidatePathIn(path, string(pathMainline), string(pathFlee), string(pathDismissed))
	if err != nil {
		return "", err
	}
	return playtestPath(parsed), nil
}
