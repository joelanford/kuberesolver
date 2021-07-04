package util

import (
	olmv1 "kuberesolver/api/v1"
)

func BundlesToCandidates(bundles []olmv1.Bundle) []olmv1.Candidate {
	out := []olmv1.Candidate{}
	for _, b := range bundles {
		out = append(out, olmv1.Candidate{
			Version:  b.Version,
			Channels: b.Channels,
			Labels:   b.Labels,
		})
	}
	return out
}
