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

// SonicDaemonSetDeployment is a specification for a SonicDaemonSetDeployment resource
type SonicDaemonSetDeployment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SonicDaemonSetDeploymentSpec   `json:"spec"`
	Status SonicDaemonSetDeploymentStatus `json:"status"`
}

// SonicDaemonSetDeploymentSpec is the spec for a SonicDaemonSetDeployment resource
type SonicDaemonSetDeploymentSpec struct {
	// ScopeType: datacenter or region
	ScopeType  string `json:"scopeType"`
	ScopeValue string `json:"scopeValue"`
	// sonic feature such as telemetry, snmp, etc
	DaemonSetType    string `json:"daemonSetType"`
	DaemonSetVersion string `json:"daemonSetVersion"`
	Pause            bool   `json:"pause"`
}

// SonicDaemonSetDeploymentStatus is the status for a SonicDaemonSetDeployment resource
type SonicDaemonSetDeploymentStatus struct {
	DesiredDaemonSetCount         int             `json:"desiredDaemonSetCount"`
	CurrentDaemonSetCount         int             `json:"currentDaemonSetCount"`
	DaemonsetList                 []DaemonSetItem `json:"daemonSetList"`
	UpdateInProgressDaemonsetList []DaemonSetItem `json:"updateInProgressDaemonSetList"`
}

type DaemonSetItem struct {
	DaemonSetName    string `json:"daemonSetName"`
	DaemonSetVersion string `json:"daemonSetVersion"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// SonicDaemonSetDeploymentList is a list of SonicDaemonSetDeployment resources
type SonicDaemonSetDeploymentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []SonicDaemonSetDeployment `json:"items"`
}
