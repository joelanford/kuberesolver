package pathselector

import (
	"context"
	"reflect"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	olmv1 "kuberesolver/api/v1"
)

type ClusterSelector struct {
	Client       client.Client
	Scheme       *runtime.Scheme
	Subscription olmv1.Subscription
}

func (s ClusterSelector) Select(ctx context.Context, req SelectorRequest) SelectorResult {
	ps, err := s.desiredPathSelector(req.Candidates)
	if err != nil {
		return SelectorResultFromError(err)
	}
	psKey := client.ObjectKeyFromObject(ps)
	createErr := s.Client.Create(ctx, ps)
	if createErr == nil {
		return SelectorResult{
			Phase:   olmv1.PhaseEvaluating,
			Message: "Waiting for path selector to make selection",
		}
	}
	if !apierrors.IsAlreadyExists(createErr) {
		return SelectorResultFromError(createErr)
	}

	if err := retry.OnError(retry.DefaultRetry, apierrors.IsNotFound, func() error {
		return s.Client.Get(ctx, psKey, ps)
	}); err != nil {
		return SelectorResultFromError(err)
	}

	desired, err := s.desiredPathSelector(req.Candidates)
	if err != nil {
		return SelectorResultFromError(err)
	}
	if !reflect.DeepEqual(ps.Spec, desired.Spec) {
		ps.Spec = desired.Spec
		if err := s.Client.Update(ctx, ps); err != nil {
			return SelectorResultFromError(err)
		}
		return SelectorResult{
			Phase:   olmv1.PhaseEvaluating,
			Message: "Waiting for path selector to make selection",
		}
	}
	if ps.Status.Phase == olmv1.PhaseEvaluating || ps.Status.Phase == "" {
		return SelectorResult{
			Phase:   olmv1.PhaseEvaluating,
			Message: "Waiting for path selector to make selection",
		}
	}

	return SelectorResult{
		Selection: ps.Status.Selection,
		Phase:     ps.Status.Phase,
		Message:   ps.Status.Message,
	}
}

func (s *ClusterSelector) desiredPathSelector(candidates []olmv1.Candidate) (*olmv1.PathSelector, error) {
	ps := &olmv1.PathSelector{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: s.Subscription.Namespace,
			Name:      s.Subscription.Name,
		},
		Spec: olmv1.PathSelectorSpec{
			PathSelectorClassName: s.Subscription.Spec.PathSelectorClassName,
			Candidates:            candidates,
		},
	}
	if err := ctrl.SetControllerReference(&s.Subscription, ps, s.Scheme); err != nil {
		return nil, err
	}
	return ps, nil
}
