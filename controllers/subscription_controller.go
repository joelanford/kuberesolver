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
	"sort"

	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	olmv1 "kuberesolver/api/v1"
	"kuberesolver/internal/pathselector"
	"kuberesolver/internal/resolver"
	"kuberesolver/internal/util"
)

// SubscriptionReconciler reconciles a Subscription object
type SubscriptionReconciler struct {
	client.Client
	NewResolverFunc func(subscription olmv1.Subscription) resolver.Resolver
	Log             logr.Logger
	Scheme          *runtime.Scheme
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
	updatedStatus := olmv1.SubscriptionStatus{
		Installed: operator.Spec.Version,
	}

	idx, pkg, err := r.getIndexAndPackage(ctx, *sub)
	if err != nil {
		return ctrl.Result{}, err
	}
	if pkg == nil {
		updatedStatus.Message = "package not found"
		updatedStatus.ResolutionPhase = olmv1.PhaseFailed
		if idx != nil {
			updatedStatus.IndexRef = indexRef(*idx)
		}
		return ctrl.Result{}, r.updateStatus(ctx, sub, updatedStatus)
	}

	resreq := resolver.Request{
		Package:    *pkg,
		Constraint: sub.Spec.Constraint,
		Installed:  operator,
	}
	resres := r.NewResolverFunc(*sub).Resolve(ctx, resreq)

	updatedStatus.Paths = &olmv1.SubscriptionPaths{}
	updatedStatus.Paths.All = util.BundlesToCandidates(pkg.Bundles)
	updatedStatus.Paths.Filtered = resres.Candidates
	updatedStatus.ResolutionPhase = resres.Phase
	updatedStatus.Message = resres.Message
	updatedStatus.UpgradeSelected = false
	updatedStatus.UpgradeAvailable = len(resres.Candidates) > 0
	if resres.Selection != nil {
		updatedStatus.UpgradeTo = resres.Selection.Version
		updatedStatus.UpgradeSelected = true
	}
	return ctrl.Result{}, r.updateStatus(ctx, sub, updatedStatus)
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

func (r *SubscriptionReconciler) updateStatus(ctx context.Context, sub *olmv1.Subscription, status olmv1.SubscriptionStatus) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		if err := r.Get(ctx, client.ObjectKeyFromObject(sub), sub); err != nil {
			return err
		}
		sub.Status = status
		return r.Status().Update(ctx, sub)
	})
}

type selectionResult struct {
	selection string
	phase     string
	message   string
	requeue   bool
}

// SetupWithManager sets up the controller with the Manager.
func (r *SubscriptionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.NewResolverFunc = func(subscription olmv1.Subscription) resolver.Resolver {
		return resolver.NewResolver(pathselector.ClusterSelector{
			Client:       mgr.GetClient(),
			Scheme:       mgr.GetScheme(),
			Subscription: subscription,
		})
	}
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
