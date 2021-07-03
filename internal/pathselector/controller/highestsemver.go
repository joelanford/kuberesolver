package controller

import (
	"context"
	"fmt"

	"github.com/blang/semver/v4"

	olmv1 "kuberesolver/api/v1"
)

const HighestSemverControllerName = "olm.operatorframework.io/highest-semver"

type HighestSemver struct{}

func (c HighestSemver) Select(_ context.Context, candidates []olmv1.Candidate, _ *olmv1.PathSelectorClassParameters) olmv1.PathSelectorStatus {
	var maxVersion *semver.Version
	for _, c := range candidates {
		version, err := semver.Parse(c.Version)
		if err != nil {
			return olmv1.PathSelectorStatus{
				Phase:   olmv1.PhaseFailed,
				Message: fmt.Sprintf("parse candidate version %q: %v", c.Version, err),
			}
		}
		if maxVersion == nil || maxVersion.LT(version) {
			maxVersion = &version
		}
	}

	if maxVersion == nil {
		return olmv1.PathSelectorStatus{
			Phase:   olmv1.PhaseFailed,
			Message: "no versioned candidates exist",
		}
	}

	return olmv1.PathSelectorStatus{
		Selection: maxVersion.String(),
		Phase:     olmv1.PhaseSucceeded,
	}
}
