/*
Copyright 2017 The Kubernetes Authors.

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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// NetworkDevice is a specification for a NetworkDevice resource
type NetworkDevice struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NetworkDeviceSpec   `json:"spec"`
	Status NetworkDeviceStatus `json:"status,omitempty"`
}

// NetworkDeviceSpec is the spec for a NetworkDevice resource
type NetworkDeviceSpec struct {
    OS OSInfo `json:"os,omitempty"`
}

type OSInfo struct {
    OSType  string `json:"os-type,omitempty"`
    Version string `json:"version,omitempty"`
}

// NetworkDeviceStatus is the status for a NetworkDevice resource
type NetworkDeviceStatus struct {
    State             string              `json:"state,omitempty"`
    LastTransitionTime string             `json:"lastTransitionTime,omitempty"`
    OS                OSInfo              `json:"os,omitempty"`
    Conditions        []Condition         `json:"conditions,omitempty"`
    Operation         *OperationStatus     `json:"operation,omitempty"`
}

// Condition represents a status condition
type Condition struct {
    Type               string      `json:"type,omitempty"`
    Status             string      `json:"status,omitempty"`
    LastProbeTime      metav1.Time `json:"lastProbeTime,omitempty"`
    LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
    Reason             string      `json:"reason,omitempty"`
    Message            string      `json:"message,omitempty"`
}

// OperationStatus describes an operation in status
type OperationStatus struct {
    Name            string           `json:"name,omitempty"`
    ID              string           `json:"id,omitempty"`
    Status          string           `json:"status,omitempty"`
    OperationAction *OperationAction  `json:"operationAction,omitempty"`
}

// OperationAction describes the action of an operation
type OperationAction struct {
    Name   string `json:"name,omitempty"`
    ID     string `json:"id,omitempty"`
    Status string `json:"status,omitempty"`
}
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// NetworkDeviceList is a list of NetworkDevice resources
// +kubebuilder:object:root=true
type NetworkDeviceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []NetworkDevice `json:"items"`
}
