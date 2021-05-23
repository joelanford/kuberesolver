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

	"github.com/blang/semver/v4"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	olmv1 "kuberesolver/api/v1"
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
	log := r.Log.WithValues("PathSelector", req.NamespacedName)
	log.Info("reconciling")

	ps := &olmv1.PathSelector{}
	if err := r.Get(ctx, req.NamespacedName, ps); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	var psc *olmv1.PathSelectorClass
	if ps.Spec.PathSelectorClassName != "" {
		pscKey := types.NamespacedName{Name: ps.Spec.PathSelectorClassName}
		if err := r.Get(ctx, pscKey, psc); err != nil {
			ps.Status.Phase = olmv1.PhaseFailed
			ps.Status.Message = err.Error()
			return ctrl.Result{}, r.Status().Update(ctx, ps)
		}
	} else {
		pscs := &olmv1.PathSelectorClassList{}
		if err := r.List(ctx, pscs); err != nil {
			ps.Status.Phase = olmv1.PhaseFailed
			ps.Status.Message = err.Error()
			return ctrl.Result{}, r.Status().Update(ctx, ps)
		}
		for _, item := range pscs.Items {
			fmt.Println(item.Annotations[olmv1.AnnotationDefaultPathSelectorClass])
			if item.Annotations[olmv1.AnnotationDefaultPathSelectorClass] == "true" {
				psc = &item
				break
			}
		}
		if psc == nil {
			ps.Status.Phase = olmv1.PhaseFailed
			ps.Status.Message = "pathSelectorClassName not defined and no default PathSelectorClass found"
			return ctrl.Result{}, r.Status().Update(ctx, ps)
		}
	}

	controller, ok := r.pathSelectorControllers()[psc.Spec.Controller]
	if !ok {
		return ctrl.Result{}, nil
	}
	newStat := controller.Select(ctx, ps.Spec.Candidates, psc.Spec.Parameters)
	if !reflect.DeepEqual(ps.Status, newStat) {
		ps.Status = newStat
		return ctrl.Result{}, r.Status().Update(ctx, ps)
	}
	return ctrl.Result{}, nil
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

func (r *PathSelectorReconciler) pathSelectorControllers() map[string]PathSelectorController {
	return map[string]PathSelectorController{
		"olm.operatorframework.io/highest-semver": &highestSemverController{},
	}
}

type PathSelectorController interface {
	Select(context.Context, []olmv1.Candidate, *olmv1.PathSelectorClassParameters) olmv1.PathSelectorStatus
}

type highestSemverController struct{}

func (c highestSemverController) Select(_ context.Context, candidates []olmv1.Candidate, pscp *olmv1.PathSelectorClassParameters) olmv1.PathSelectorStatus {
	var maxVersion *semver.Version
	for _, c := range candidates {
		version, err := semver.Parse(c.Version)
		if err != nil {
			return olmv1.PathSelectorStatus{
				Phase:   olmv1.PhaseFailed,
				Message: fmt.Sprintf("parse candidate version %q: %v", c.Version, err),
			}
		}
		if maxVersion == nil || maxVersion.LT(version) {
			maxVersion = &version
		}
	}

	if maxVersion == nil {
		return olmv1.PathSelectorStatus{
			Phase:   olmv1.PhaseFailed,
			Message: "no versioned candidates exist",
		}
	}

	return olmv1.PathSelectorStatus{
		Selection: maxVersion.String(),
		Phase:     olmv1.PhaseSucceeded,
	}
}
