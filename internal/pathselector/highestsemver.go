package pathselector

import (
	"context"
	"fmt"

	"github.com/blang/semver/v4"

	olmv1 "kuberesolver/api/v1"
)

const HighestSemverControllerName = "olm.operatorframework.io/highest-semver"

type HighestSemverController struct{}

func (c HighestSemverController) Select(ctx context.Context, candidates []olmv1.Candidate, _ *olmv1.PathSelectorClassParameters) SelectorResult {
	return HighestSemverSelector{}.Select(ctx, SelectorRequest{Candidates: candidates})
}

type HighestSemverSelector struct{}

func (s HighestSemverSelector) Select(_ context.Context, req SelectorRequest) SelectorResult {
	var (
		maxVersion *semver.Version
		selection  *olmv1.Candidate
	)
	for _, c := range req.Candidates {
		version, err := semver.Parse(c.Version)
		if err != nil {
			return SelectorResult{
				Phase:   olmv1.PhaseFailed,
				Message: fmt.Sprintf("parse candidate version %q: %v", c.Version, err),
			}
		}
		if maxVersion == nil || maxVersion.LT(version) {
			maxVersion = &version
			selection = &c
		}
	}

	if maxVersion == nil {
		return SelectorResult{
			Phase:   olmv1.PhaseFailed,
			Message: "no versioned candidates exist",
		}
	}

	return SelectorResult{
		Selection: selection,
		Phase:     olmv1.PhaseSucceeded,
	}
}
