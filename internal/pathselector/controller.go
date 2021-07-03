package pathselector

import (
	"context"

	olmv1 "kuberesolver/api/v1"
	"kuberesolver/internal/pathselector/controller"
)

type Controller interface {
	Select(context.Context, []olmv1.Candidate, *olmv1.PathSelectorClassParameters) olmv1.PathSelectorStatus
}

var Controllers = map[string]Controller{
	controller.HighestSemverControllerName: &controller.HighestSemver{},
}
