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
	"reflect"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	olmv1 "kuberesolver/api/v1"
	"kuberesolver/internal/pathselector"
)

// PathSelectorReconciler reconciles a PathSelector object
type PathSelectorReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=olm.operatorframework.io,resources=pathselectors,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=olm.operatorframework.io,resources=pathselectors/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=olm.operatorframework.io,resources=pathselectors/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the PathSelector object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r *PathSelectorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("pathSelector", req.NamespacedName)
	log.Info("reconciling")

	ps := &olmv1.PathSelector{}
	if err := r.Get(ctx, req.NamespacedName, ps); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	psc, err := r.getPathSelectorClass(ctx, ps.Spec.PathSelectorClassName)
	if err != nil {
		return ctrl.Result{}, err
	}
	if psc == nil {
		newStat := olmv1.PathSelectorStatus{
			Message: "Path selector class not specified and no default path selector class present on cluster",
			Phase:   olmv1.PhaseFailed,
		}
		if !reflect.DeepEqual(ps.Status, newStat) {
			ps.Status = newStat
			return ctrl.Result{}, r.Status().Update(ctx, ps)
		}
		return ctrl.Result{}, nil
	}

	controller, ok := pathselector.Controllers[psc.Spec.Controller]
	if !ok {
		// If the controller specified by the psc is not known to us, stop
		// reconciling. This isn't our path selector to reconcile.
		return ctrl.Result{}, nil
	}
	res := controller.Select(ctx, ps.Spec.Candidates, psc.Spec.Parameters)
	newStat := olmv1.PathSelectorStatus{
		Phase:   res.Phase,
		Message: res.Message,
	}
	if res.Selection != nil {
		newStat.Selection = res.Selection
	}
	if !reflect.DeepEqual(ps.Status, newStat) {
		ps.Status = newStat
		return ctrl.Result{}, r.Status().Update(ctx, ps)
	}
	return ctrl.Result{}, nil
}

func (r *PathSelectorReconciler) getPathSelectorClass(ctx context.Context, pscName string) (*olmv1.PathSelectorClass, error) {
	// If the path selector class name is non-empty, get and return it
	if pscName != "" {
		psc := &olmv1.PathSelectorClass{}
		pscKey := types.NamespacedName{Name: pscName}
		if err := r.Get(ctx, pscKey, psc); err != nil {
			return nil, err
		}
		return psc, nil
	}

	// Otherwise, we'll only reconcile this path selector if one of the path
	// selector classes that we own is set as the default.
	pscList := &olmv1.PathSelectorClassList{}
	if err := r.List(ctx, pscList); err != nil {
		return nil, err
	}
	for _, psc := range pscList.Items {
		if psc.IsDefault() {
			return &psc, nil
		}
	}

	// If the path selector does not specify a path selector class and there is
	// not a default path selector class, no path selector controller will
	// reconcile this path selector. This is not an error condition, but there
	// also isn't a PSC to return. Callers should update the PS status to inform
	// the PS creator about this condition.
	return nil, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *PathSelectorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&olmv1.PathSelector{}).
		Watches(&source.Kind{Type: &olmv1.PathSelectorClass{}}, handler.EnqueueRequestsFromMapFunc(func(obj client.Object) []reconcile.Request {
			psc := obj.(*olmv1.PathSelectorClass)
			pss := &olmv1.PathSelectorList{}
			if err := r.List(context.TODO(), pss); err != nil {
				return nil
			}
			reqs := []reconcile.Request{}
			for _, item := range pss.Items {
				if item.Spec.PathSelectorClassName == psc.Name || item.Spec.PathSelectorClassName == "" {
					key := client.ObjectKeyFromObject(&item)
					reqs = append(reqs, reconcile.Request{NamespacedName: key})
				}
			}
			return reqs
		})).
		Complete(r)
}

type PathSelectorController interface {
	Select(context.Context, []olmv1.Candidate, *olmv1.PathSelectorClassParameters) olmv1.PathSelectorStatus
}
