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

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	annotationDefaultPathSelectorClass = "pathselectorclass.olm.operatorframework.io/is-default-class"
)

// PathSelectorClassSpec defines the desired state of PathSelectorClass
type PathSelectorClassSpec struct {
	// Controller refers to the name of the controller that should handle this
	// class. This allows for different "flavors" that are controlled by the same
	// controller. For example, you may have different Parameters for the same
	// implementing controller. This should be specified as a domain-prefixed path
	// no more than 250 characters in length, e.g. "acme.io/path-selector-controller".
	// This field is immutable.
	Controller string `json:"controller"`

	// Parameters is a link to a custom resource containing additional
	// configuration for the controller. This is optional if the controller does
	// not require extra parameters.
	Parameters *PathSelectorClassParameters `json:"parameters,omitempty"`
}

// PathSelectorClassParameters defines a reference to an object containing
// configuration for a path selector controller.
type PathSelectorClassParameters struct {
	// APIGroup is the group for the resource being referenced. If APIGroup is not
	// specified, the specified Kind must be in the core API group. For any other
	// third-party types, APIGroup is required.
	APIGroup string `json:"apiGroup"`

	// Kind is the type of resource being referenced
	Kind string `json:"kind"`

	// Name is the name of resource being referenced
	Name string `json:"name"`
}

//+kubebuilder:object:root=true
//+kubebuilder:resource:scope=Cluster

// PathSelectorClass is the Schema for the pathselectorclasses API
type PathSelectorClass struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec PathSelectorClassSpec `json:"spec,omitempty"`
}

func (psc PathSelectorClass) IsDefault() bool {
	return psc.Annotations[annotationDefaultPathSelectorClass] == "true"
}

//+kubebuilder:object:root=true

// PathSelectorClassList contains a list of PathSelectorClass
type PathSelectorClassList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PathSelectorClass `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PathSelectorClass{}, &PathSelectorClassList{})
}
