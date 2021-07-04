package pathselector

import (
	"context"

	olmv1 "kuberesolver/api/v1"
)

type Controller interface {
	Select(context.Context, []olmv1.Candidate, *olmv1.PathSelectorClassParameters) SelectorResult
}

var Controllers = map[string]Controller{
	HighestSemverControllerName: &HighestSemverController{},
}
