package resolver

import (
	"context"
	"fmt"

	olmv1 "kuberesolver/api/v1"
	"kuberesolver/internal/pathselector"
	"kuberesolver/internal/util"
)

type Request struct {
	Package      olmv1.Package
	Constraint   *olmv1.Constraint
	Installed    *olmv1.Operator
	PathSelector pathselector.Selector
}

type Result struct {
	Candidates []olmv1.Candidate
	Selection  *olmv1.Candidate
	Phase      string
	Message    string
}

type Resolver interface {
	Resolve(context.Context, Request) Result
}

func NewResolver(ps pathselector.Selector) Resolver {
	return &resolver{PathSelector: ps}
}

type resolver struct {
	PathSelector pathselector.Selector
}

func (r resolver) Resolve(ctx context.Context, request Request) Result {
	res := Result{}
	all := allCandidateBundles(request.Package, request.Installed.Spec.Version)
	filtered, err := constrainCandidateBundles(all, request.Constraint)
	if err != nil {
		res.Message = fmt.Sprintf("Failed to apply constraint: %v", err)
		res.Phase = olmv1.PhaseFailed
		return res
	}
	candidates := util.BundlesToCandidates(filtered)
	res.Candidates = candidates

	switch len(candidates) {
	case 0:
		res.Phase = olmv1.PhaseSucceeded
		res.Message = "No upgrades available"
	case 1:
		res.Phase = olmv1.PhaseSucceeded
		res.Message = "Found 1 candidate that matches constraints"
		res.Selection = &candidates[0]
	default:
		sr := r.PathSelector.Select(ctx, pathselector.SelectorRequest{Candidates: candidates})
		res.Phase = sr.Phase
		res.Message = sr.Message
		res.Selection = sr.Selection
	}
	return res
}

func allCandidateBundles(pkg olmv1.Package, installed string) []olmv1.Bundle {
	bundles := pkg.Bundles
	if installed != "" {
		bundles = []olmv1.Bundle{}
		for _, b := range pkg.Bundles {
			for _, from := range b.UpgradesFrom {
				if from == installed {
					bundles = append(bundles, b)
				}
			}
		}
	}
	return bundles
}

func constrainCandidateBundles(bundles []olmv1.Bundle, constraint *olmv1.Constraint) ([]olmv1.Bundle, error) {
	if constraint != nil {
		var err error
		bundles, err = constraint.Apply(bundles)
		if err != nil {
			return nil, fmt.Errorf("apply subscription constraint to candidates: %v", err)
		}
	}
	return bundles, nil
}
