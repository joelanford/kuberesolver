/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"
	"reflect"
	"sort"

	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	olmv1 "kuberesolver/api/v1"
)

// SubscriptionReconciler reconciles a Subscription object
type SubscriptionReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=olm.operatorframework.io,resources=subscriptions,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=olm.operatorframework.io,resources=subscriptions/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=olm.operatorframework.io,resources=subscriptions/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Subscription object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r *SubscriptionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("subscription", req.NamespacedName)
	log.Info("reconciling")

	sub := &olmv1.Subscription{}
	if err := r.Get(ctx, req.NamespacedName, sub); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	sub.Status = olmv1.SubscriptionStatus{}

	operatorKey := types.NamespacedName{
		Namespace: sub.Namespace,
		Name:      sub.Spec.Package,
	}
	operator := &olmv1.Operator{}
	if err := r.Get(ctx, operatorKey, operator); err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
	}
	sub.Status.Installed = operator.Spec.Version

	idx, pkg, err := r.getIndexAndPackage(ctx, *sub)
	if err != nil {
		return ctrl.Result{}, err
	}
	if pkg == nil {
		sub.Status = olmv1.SubscriptionStatus{}
		if idx != nil {
			sub.Status.IndexRef = indexRef(*idx)
		}
		sub.Status.Message = "package not found"
		sub.Status.ResolutionPhase = olmv1.PhaseFailed
		return ctrl.Result{}, r.Status().Update(ctx, sub)
	}

	return ctrl.Result{}, r.resolve(ctx, sub, *pkg)
}

func indexRef(idx olmv1.Index) *v1.ObjectReference {
	return &v1.ObjectReference{
		Kind:            idx.Kind,
		Namespace:       idx.Namespace,
		Name:            idx.Name,
		UID:             idx.UID,
		APIVersion:      idx.APIVersion,
		ResourceVersion: idx.ResourceVersion,
	}
}

func (r *SubscriptionReconciler) getIndexAndPackage(ctx context.Context, sub olmv1.Subscription) (*olmv1.Index, *olmv1.Package, error) {
	if sub.Spec.Index != "" {
		idx := &olmv1.Index{}
		if err := r.Get(ctx, types.NamespacedName{Name: sub.Spec.Index}, idx); err != nil {
			return nil, nil, err
		}
		for _, p := range idx.Spec.Packages {
			if p.Name == sub.Spec.Package {
				return idx, &p, nil
			}
		}
		return idx, nil, nil
	}

	idxList := &olmv1.IndexList{}
	if err := r.List(ctx, idxList); err != nil {
		return nil, nil, err
	}
	sort.Slice(idxList.Items, func(i, j int) bool {
		if idxList.Items[i].Spec.Priority != idxList.Items[j].Spec.Priority {
			return idxList.Items[i].Spec.Priority < idxList.Items[j].Spec.Priority
		}
		return idxList.Items[i].Name < idxList.Items[j].Name
	})
	for _, idx := range idxList.Items {
		for _, p := range idx.Spec.Packages {
			if p.Name == sub.Spec.Package {
				return &idx, &p, nil
			}
		}
	}
	return nil, nil, nil
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

func (r *SubscriptionReconciler) resolve(ctx context.Context, sub *olmv1.Subscription, pkg olmv1.Package) error {
	sub.Status.Paths = &olmv1.SubscriptionPaths{}

	all := allCandidateBundles(pkg, sub.Status.Installed)
	sub.Status.Paths.All = bundlesToCandidates(all...)

	filtered, err := constrainCandidateBundles(all, sub.Spec.Constraint)
	if err != nil {
		return fmt.Errorf("apply candidate constraints from subscription: %v", err)
	}
	candidates := bundlesToCandidates(filtered...)
	sub.Status.Paths.Filtered = candidates
	sub.Status.UpgradeAvailable = len(candidates) > 0

	switch len(candidates) {
	case 0:
		sub.Status.ResolutionPhase = olmv1.PhaseSucceeded
		sub.Status.Message = "No upgrades available"
	case 1:
		sub.Status.ResolutionPhase = olmv1.PhaseSucceeded
		sub.Status.Message = "Found 1 candidate that matches constraints"
		sub.Status.UpgradeTo = candidates[0].Version
		sub.Status.UpgradeSelected = true
	default:
		res, err := r.selectPath(ctx, *sub, candidates)
		if err != nil {
			return err
		}
		sub.Status.ResolutionPhase = res.phase
		sub.Status.Message = res.message
		sub.Status.UpgradeTo = res.selection
		sub.Status.UpgradeSelected = res.selection != ""
	}
	return r.Status().Update(ctx, sub)
}

func bundlesToCandidates(bundles ...olmv1.Bundle) []olmv1.Candidate {
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

type selectionResult struct {
	selection string
	phase     string
	message   string
	requeue   bool
}

func (r *SubscriptionReconciler) selectPath(ctx context.Context, sub olmv1.Subscription, candidates []olmv1.Candidate) (*selectionResult, error) {
	ps, err := r.desiredPathSelector(sub, candidates)
	if err != nil {
		return nil, err
	}
	psKey := client.ObjectKeyFromObject(ps)
	createErr := r.Create(ctx, ps)
	if createErr == nil {
		return &selectionResult{
			phase:   olmv1.PhaseEvaluating,
			message: "Waiting for path selector to make selection",
		}, nil
	}
	if !apierrors.IsAlreadyExists(createErr) {
		return nil, createErr
	}

	if err := retry.OnError(retry.DefaultRetry, apierrors.IsNotFound, func() error {
		return r.Get(ctx, psKey, ps)
	}); err != nil {
		return nil, err
	}

	desired, err := r.desiredPathSelector(sub, candidates)
	if err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(ps.Spec, desired.Spec) {
		ps.Spec = desired.Spec
		if err := r.Update(ctx, ps); err != nil {
			return nil, err
		}
		return &selectionResult{
			phase:   olmv1.PhaseEvaluating,
			message: "Waiting for path selector to make selection",
		}, nil
	}
	if ps.Status.Phase == olmv1.PhaseEvaluating || ps.Status.Phase == "" {
		return &selectionResult{
			phase:   olmv1.PhaseEvaluating,
			message: "Waiting for path selector to make selection",
		}, nil
	}

	res := &selectionResult{
		selection: ps.Status.Selection,
		phase:     ps.Status.Phase,
		message:   ps.Status.Message,
	}
	return res, nil
}

func (r *SubscriptionReconciler) desiredPathSelector(sub olmv1.Subscription, candidates []olmv1.Candidate) (*olmv1.PathSelector, error) {
	ps := &olmv1.PathSelector{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: sub.Namespace,
			Name:      sub.Name,
		},
		Spec: olmv1.PathSelectorSpec{
			PathSelectorClassName: sub.Spec.PathSelectorClassName,
			Candidates:            candidates,
		},
	}
	if err := ctrl.SetControllerReference(&sub, ps, r.Scheme); err != nil {
		return nil, err
	}
	return ps, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *SubscriptionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&olmv1.Subscription{}).
		Owns(&olmv1.PathSelector{}).
		Watches(&source.Kind{Type: &olmv1.PathSelectorClass{}}, handler.EnqueueRequestsFromMapFunc(func(obj client.Object) []reconcile.Request {
			subs := &olmv1.SubscriptionList{}
			if err := r.List(context.TODO(), subs); err != nil {
				return nil
			}
			reqs := []reconcile.Request{}
			for _, item := range subs.Items {
				key := client.ObjectKeyFromObject(&item)
				reqs = append(reqs, reconcile.Request{NamespacedName: key})
			}
			return reqs
		})).
		Watches(&source.Kind{Type: &olmv1.Operator{}}, handler.EnqueueRequestsFromMapFunc(func(obj client.Object) []reconcile.Request {
			op := obj.(*olmv1.Operator)
			subs := &olmv1.SubscriptionList{}
			if err := r.List(context.TODO(), subs); err != nil {
				return nil
			}
			reqs := []reconcile.Request{}
			for _, item := range subs.Items {
				if item.Spec.Package == op.Spec.Package {
					key := client.ObjectKeyFromObject(&item)
					reqs = append(reqs, reconcile.Request{NamespacedName: key})
				}
			}
			return reqs
		})).
		Watches(&source.Kind{Type: &olmv1.Index{}}, handler.EnqueueRequestsFromMapFunc(func(client.Object) []reconcile.Request {
			subs := &olmv1.SubscriptionList{}
			if err := r.List(context.TODO(), subs); err != nil {
				return nil
			}
			reqs := []reconcile.Request{}
			for _, item := range subs.Items {
				key := client.ObjectKeyFromObject(&item)
				reqs = append(reqs, reconcile.Request{NamespacedName: key})
			}
			return reqs
		})).
		Complete(r)
}
