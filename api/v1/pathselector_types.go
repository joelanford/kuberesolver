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

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// PathSelectorSpec defines the desired state of PathSelector
type PathSelectorSpec struct {
	// The name of the path selector class that should respond to this path
	// selector.
	//+kubebuilder:validation:Optional
	PathSelectorClassName string `json:"pathSelectorClassName,omitempty"`

	// Candidates is a list of candidate choices for a path selector to select
	// from.
	Candidates []Candidate `json:"candidates"`
}

// PathSelectorStatus defines the observed state of PathSelector
type PathSelectorStatus struct {
	// Selection is the name of the candidate chosen by a PathSelector controller.
	Selection string `json:"selection,omitempty"`

	// Phase is a simple CamelCase string that describes the state of the
	// path selection. Possibilities are: Evaluating, Succeeded, Failed
	Phase string `json:"phase,omitempty"`

	// Message is an arbitrary string that provides further detail and context
	// about the current phase.
	Message string `json:"message,omitempty"`
}

const (
	PhaseEvaluating = "Evaluating"
	PhaseSucceeded  = "Succeeded"
	PhaseFailed     = "Failed"
)

// Candidate contains a unique name for a resolution candidate and all of its
// properties. A path selector evaluates candidate properties and reports the
// name of the candidate that is chosen.
type Candidate struct {

	// Version is the version of the candidate within the global set of all
	// candidates of a particular package unique identifier for a candidate. Uniqueness is
	// determined by the context of clients of the PathSelector API.
	Version string `json:"version"`

	// Channels is a list of channels that the candidate is a member of.
	// The PathSelector may evaluate these channels to make a selection.
	//+kubebuilder:validation:Optional
	Channels []string `json:"channels"`

	// Labels is a map of key/value pairs  that describe the candidate.
	// The PathSelector may evaluate these labels to make a selection.
	//+kubebuilder:validation:Optional
	Labels map[string]string `json:"labels"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// PathSelector is the Schema for the pathselectors API
type PathSelector struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PathSelectorSpec   `json:"spec,omitempty"`
	Status PathSelectorStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// PathSelectorList contains a list of PathSelector
type PathSelectorList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PathSelector `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PathSelector{}, &PathSelectorList{})
}
