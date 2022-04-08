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
	"sort"

	"github.com/go-logr/logr"
	rukpakv1alpha1 "github.com/operator-framework/rukpak/api/v1alpha1"
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

	bundleInstanceKey := types.NamespacedName{
		Name: sub.Spec.Package,
	}
	bundleInstance := &rukpakv1alpha1.BundleInstance{}
	if err := r.Get(ctx, bundleInstanceKey, bundleInstance); err != nil {
		if !apierrors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
	}
	updatedStatus := olmv1.SubscriptionStatus{
		Installed: bundleInstance.Status.InstalledBundleName,
	}

	pkg, err := r.getPackage(ctx, *sub)
	if err != nil {
		return ctrl.Result{}, err
	}
	if pkg == nil {
		updatedStatus.Message = "package not found"
		updatedStatus.ResolutionPhase = olmv1.PhaseFailed
		return ctrl.Result{}, r.updateStatus(ctx, sub, updatedStatus)
	}

	resreq := resolver.Request{
		Package:    *pkg,
		Constraint: sub.Spec.Constraint,
		Installed:  bundleInstance,
	}
	resres := r.NewResolverFunc(*sub).Resolve(ctx, resreq)

	possibleBundles := possible(pkg, bundleInstance.Status.InstalledBundleName)

	updatedStatus.Paths = &olmv1.SubscriptionPaths{}
	updatedStatus.Paths.All = util.BundlesToCandidates(possibleBundles)
	updatedStatus.Paths.Filtered = resres.Candidates
	updatedStatus.ResolutionPhase = resres.Phase
	updatedStatus.Message = resres.Message
	updatedStatus.UpgradeSelected = false
	updatedStatus.UpgradeAvailable = len(resres.Candidates) > 0
	if resres.Selection != nil {
		updatedStatus.UpgradeTo = resres.Selection.Version
		updatedStatus.UpgradeSelected = true

		bundle := &rukpakv1alpha1.Bundle{
			ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("%s-v%s", resreq.Package.Name, resres.Selection.Version),
			},
			Spec: rukpakv1alpha1.BundleSpec{
				ProvisionerClassName: "core.rukpak.io/plain",
				Source: rukpakv1alpha1.BundleSource{
					Type: rukpakv1alpha1.SourceTypeImage,
					Image: &rukpakv1alpha1.ImageSource{
						Ref: resres.Selection.Image,
					},
				},
			},
		}
		if err := r.Create(ctx, bundle); err != nil && !apierrors.IsAlreadyExists(err) {
			return ctrl.Result{}, err
		}

		bundleInstance := &rukpakv1alpha1.BundleInstance{
			ObjectMeta: metav1.ObjectMeta{
				Name: resreq.Package.Name,
			},
			Spec: rukpakv1alpha1.BundleInstanceSpec{
				ProvisionerClassName: "core.rukpak.io/plain",
				BundleName:           bundle.Name,
			},
		}
		err := r.Get(ctx, client.ObjectKeyFromObject(bundleInstance), bundleInstance)
		if err != nil {
			if !apierrors.IsNotFound(err) {
				return ctrl.Result{}, err
			}
			if err := r.Create(ctx, bundleInstance); err != nil {
				return ctrl.Result{}, err
			}
		} else {
			oldBundleName := bundleInstance.Spec.BundleName
			bundleInstance.Spec.BundleName = bundle.Name
			bundleInstance.Spec.ProvisionerClassName = "core.rukpak.io/plain"
			if err := r.Update(ctx, bundleInstance); err != nil {
				return ctrl.Result{}, err
			}
			if oldBundleName != bundle.Name {
				deleteBundle := &rukpakv1alpha1.Bundle{
					ObjectMeta: metav1.ObjectMeta{
						Name: oldBundleName,
					},
				}
				if err := r.Delete(ctx, deleteBundle); client.IgnoreNotFound(err) != nil {
					return ctrl.Result{}, err
				}
			}
		}

	}
	return ctrl.Result{}, r.updateStatus(ctx, sub, updatedStatus)
}

// getPackage returns the package specified by the subscription. If the subscription specifies an index,
// the package from that index is returned. If the subscription does not specify an index, a package is
// generated by combining the matching packages from each index in the cluster. If multiple indexes contain
// the same version of a bundle in the specified package, the bundle from the index with the highest priority
// will be present in the returned package. If two indexes with the same priority share a bundle with the same
// version, the bundle will be chosen from the index whose name sorts highest alphanumerically.
func (r *SubscriptionReconciler) getPackage(ctx context.Context, sub olmv1.Subscription) (*olmv1.Package, error) {
	if sub.Spec.Index != "" {
		idx := &olmv1.Index{}
		if err := r.Get(ctx, types.NamespacedName{Name: sub.Spec.Index}, idx); err != nil {
			return nil, err
		}
		for _, p := range idx.Spec.Packages {
			if p.Name == sub.Spec.Package {
				return &p, nil
			}
		}
		// Nil, nil means "couldn't find this package"
		return nil, nil
	}

	idxList := &olmv1.IndexList{}
	if err := r.List(ctx, idxList); err != nil {
		return nil, err
	}

	// Sort by priority such that lower priority packages are encountered first.
	sort.Slice(idxList.Items, func(i, j int) bool {
		if idxList.Items[i].Spec.Priority != idxList.Items[j].Spec.Priority {
			return idxList.Items[i].Spec.Priority < idxList.Items[j].Spec.Priority
		}
		return idxList.Items[i].Name < idxList.Items[j].Name
	})

	// Iterate the indexes, building a bundle map, overwriting duplicates as we
	// iterate, guaranteeing that the highest priority bundles end up in the
	// returned package.
	var pkg *olmv1.Package
	bundleMap := map[string]olmv1.Bundle{}
	for _, idx := range idxList.Items {
		for _, p := range idx.Spec.Packages {
			if p.Name == sub.Spec.Package {
				if pkg == nil {
					pkg = &olmv1.Package{Name: sub.Spec.Package}
				}
				for _, b := range p.Bundles {
					bundleMap[b.Version] = b
				}
			}
		}
	}
	if pkg == nil {
		return nil, nil
	}

	// Now that the bundles have been added and deduplicated from each index and
	// package, add them to the generated package.
	for _, b := range bundleMap {
		pkg.Bundles = append(pkg.Bundles, b)
	}
	// IMPORTANT: Sort the returned bundles so that the order is consistent to
	// prevent hot loops due to constant reconcilation caused by different list
	// orders.
	sort.Slice(pkg.Bundles, func(i, j int) bool {
		return pkg.Bundles[i].Version < pkg.Bundles[j].Version
	})
	return pkg, nil
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
			psc := obj.(*olmv1.PathSelectorClass)
			subs := &olmv1.SubscriptionList{}
			if err := r.List(context.TODO(), subs); err != nil {
				return nil
			}
			reqs := []reconcile.Request{}
			for _, item := range subs.Items {
				if (psc.IsDefault() && item.Spec.PathSelectorClassName == "") || (psc.Name == item.Spec.PathSelectorClassName) {
					key := client.ObjectKeyFromObject(&item)
					reqs = append(reqs, reconcile.Request{NamespacedName: key})
				}
			}
			return reqs
		})).
		Watches(&source.Kind{Type: &rukpakv1alpha1.BundleInstance{}}, handler.EnqueueRequestsFromMapFunc(func(obj client.Object) []reconcile.Request {
			op := obj.(*rukpakv1alpha1.BundleInstance)
			subs := &olmv1.SubscriptionList{}
			if err := r.List(context.TODO(), subs); err != nil {
				return nil
			}
			reqs := []reconcile.Request{}
			for _, item := range subs.Items {
				if item.Spec.Package == op.Name {
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

func possible(pkg *olmv1.Package, installed string) []olmv1.Bundle {
	if installed == "" {
		return pkg.Bundles
	}
	filtered := []olmv1.Bundle{}
	for _, b := range pkg.Bundles {
		for _, uf := range b.UpgradesFrom {
			if fmt.Sprintf("%s-v%s", pkg.Name, uf) == installed {
				filtered = append(filtered, b)
				break
			}
		}
	}
	return filtered
}
