package pathselector

import (
	"context"

	olmv1 "kuberesolver/api/v1"
)

type SelectorRequest struct {
	Candidates []olmv1.Candidate
}

type SelectorResult struct {
	Selection *olmv1.Candidate
	Phase     string
	Message   string
}

type Selector interface {
	Select(context.Context, SelectorRequest) SelectorResult
}

func SelectorResultFromError(err error) SelectorResult {
	return SelectorResult{
		Phase:   olmv1.PhaseFailed,
		Message: err.Error(),
	}
}
