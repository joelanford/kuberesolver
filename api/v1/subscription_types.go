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
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// SubscriptionSpec defines the desired state of Subscription
type SubscriptionSpec struct {
	//+kubebuilder:validation:Optional
	Index string `json:"index,omitempty"`

	//+kubebuilder:validation:Required
	Package string `json:"package"`

	//+kubebuilder:validation:Enum=Automatic;Manual
	//+kubebuilder:default:=Automatic
	//+kubebuilder:validation:Required
	Approval string `json:"approval"`

	//+kubebuilder:validation:Optional
	Constraint *Constraint `json:"constraint,omitempty"`

	//+kubebuilder:validation:Optional
	PathSelectorClassName string `json:"pathSelectorClassName,omitempty"`
}

type Constraint struct {
	All      []Constraint          `json:"all,omitempty"`
	Any      []Constraint          `json:"any,omitempty"`
	Negate   *bool                 `json:"negate,omitempty"`
	Channel  string                `json:"channel,omitempty"`
	Selector *metav1.LabelSelector `json:"selector,omitempty"`
}

// SubscriptionStatus defines the observed state of Subscription
type SubscriptionStatus struct {
	IndexRef         *v1.ObjectReference `json:"indexReference,omitempty"`
	Paths            *SubscriptionPaths  `json:"paths,omitempty"`
	UpgradeAvailable bool                `json:"upgradeAvailable"`
	UpgradeSelected  bool                `json:"upgradeSelected"`
	UpgradeTo        string              `json:"upgradeTo,omitempty"`
	Installed        string              `json:"installed,omitempty"`
	ResolutionPhase  string              `json:"resolutionPhase,omitempty"`
	Message          string              `json:"message,omitempty"`
}

type SubscriptionPaths struct {
	All      []Candidate `json:"all,omitempty"`
	Filtered []Candidate `json:"filtered,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Package",type=string,JSONPath=`.spec.package`
// +kubebuilder:printcolumn:name="Installed",type=string,JSONPath=`.status.installed`
// +kubebuilder:printcolumn:name="Approval",type=string,JSONPath=`.spec.approval`
// +kubebuilder:printcolumn:name="Upgrade Available",type=boolean,JSONPath=`.status.upgradeAvailable`
// +kubebuilder:printcolumn:name="Upgrade Selected",type=boolean,JSONPath=`.status.upgradeSelected`
// +kubebuilder:printcolumn:name="Next Version",type=string,JSONPath=`.status.upgradeTo`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// Subscription is the Schema for the subscriptions API
type Subscription struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SubscriptionSpec   `json:"spec,omitempty"`
	Status SubscriptionStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// SubscriptionList contains a list of Subscription
type SubscriptionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Subscription `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Subscription{}, &SubscriptionList{})
}

func (c Constraint) Apply(bundles []Bundle) ([]Bundle, error) {
	out := []Bundle{}
	for _, b := range bundles {
		if isMatch, err := c.Match(b); err != nil {
			return nil, err
		} else if isMatch {
			out = append(out, b)
		}
	}
	return out, nil
}

func (c Constraint) Match(b Bundle) (bool, error) {
	negate := false
	if c.Negate != nil {
		negate = *c.Negate
	}
	matchFuncs := []func(b Bundle) (bool, error){}
	if len(c.All) > 0 {
		matchFuncs = append(matchFuncs, c.matchAll)
	}
	if len(c.Any) > 0 {
		matchFuncs = append(matchFuncs, c.matchAny)
	}
	if len(c.Channel) > 0 {
		matchFuncs = append(matchFuncs, c.matchChannel)
	}
	if c.Selector != nil {
		matchFuncs = append(matchFuncs, c.matchSelector)
	}
	for _, match := range matchFuncs {
		isMatch, err := match(b)
		if err != nil {
			return false, err
		}
		// negate XNOR match(b)
		//   explanation: XNOR on booleans is the same as ==, either both are
		//   false or both are true.
		if negate == isMatch {
			return false, nil
		}
	}
	return true, nil
}

func (c Constraint) matchAll(b Bundle) (bool, error) {
	for _, subc := range c.All {
		if isMatch, err := subc.Match(b); !isMatch || err != nil {
			return false, err
		}
	}
	return true, nil
}

func (c Constraint) matchAny(b Bundle) (bool, error) {
	for _, a := range c.Any {
		if isMatch, err := a.Match(b); isMatch || err != nil {
			return isMatch, err
		}
	}
	return false, nil
}

func (c Constraint) matchChannel(b Bundle) (bool, error) {
	for _, ch := range b.Channels {
		if ch == c.Channel {
			return true, nil
		}
	}
	return false, nil
}

func (c Constraint) matchSelector(b Bundle) (bool, error) {
	sel, err := metav1.LabelSelectorAsSelector(c.Selector)
	if err != nil {
		return false, err
	}
	if !sel.Matches(b.Labels) {
		return false, nil
	}
	return true, nil
}
