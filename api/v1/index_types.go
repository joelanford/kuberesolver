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
	"k8s.io/apimachinery/pkg/labels"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// IndexSpec defines the desired state of Index
type IndexSpec struct {
	// The priority of this index when choosing a package that exists in multiple indices. Lower numbers mean
	// lower priority. For example, packages in an index with priority 0 (or unset priority) are the lowest priority,
	// and packages with increasing values are higher and higher priority.
	Priority uint      `json:"priority"`
	Packages []Package `json:"packages,omitempty"`
}

type Package struct {
	Name    string   `json:"name"`
	Bundles []Bundle `json:"bundles,omitempty"`
}

type Bundle struct {
	Image        string     `json:"image"`
	Version      string     `json:"version"`
	UpgradesFrom []string   `json:"upgradesFrom,omitempty"`
	Channels     []string   `json:"channels,omitempty"`
	Labels       labels.Set `json:"labels,omitempty"`
}

// IndexStatus defines the observed state of Index
type IndexStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:scope=Cluster
//+kubebuilder:printcolumn:name="Priority",type=string,JSONPath=`.spec.priority`
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Index is the Schema for the indices API
type Index struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   IndexSpec   `json:"spec,omitempty"`
	Status IndexStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// IndexList contains a list of Index
type IndexList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Index `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Index{}, &IndexList{})
}
