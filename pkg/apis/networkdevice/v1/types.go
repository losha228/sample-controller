/*
Copyright 2025 The Kubernetes Authors.

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

type NetworkDeviceState string

const (
	// network device states
    NetworkDeviceStateHealthy  NetworkDeviceState     = "Healthy"
    NetworkDeviceStateOffline   NetworkDeviceState     = "Offline"
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
    // device type of the network device
    Type      string `json:"type,omitempty"`
    OsVersion string `json:"osVersion,omitempty"`
    FirmwareProfile string `json:"firmwareProfile,omitempty"`
    Operation string `json:"operation,omitempty"`
    OperationAction string `json:"operationAction,omitempty"`
}

// NetworkDeviceStatus is the status for a NetworkDevice resource
type NetworkDeviceStatus struct {
    // current state of the network device
    State             string              `json:"state,omitempty"`
    // last transition time of the network device state
    LastTransitionTime metav1.Time         `json:"lastTransitionTime,omitempty"`
    // current OS version of the network device
    OsVersion         string              `json:"osVersion,omitempty"`
    // current operation state of the network device
    OperationState         string              `json:"operationState,omitempty"`
    // current operation action state of the network device
    OperationActionState string              `json:"operationActionState,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// NetworkDeviceList is a list of NetworkDevice resources
// +kubebuilder:object:root=true
type NetworkDeviceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []NetworkDevice `json:"items"`
}
