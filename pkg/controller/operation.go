package controller

import (
	samplev1alpha1 "k8s.io/sample-controller/pkg/apis/sonic/v1alpha1"
)

// define an interface for operation handling
type OperationHandler interface {
	Proceed(device *samplev1alpha1.NetworkDevice) bool
	NextAction(device *samplev1alpha1.NetworkDevice) (string, bool)
}

type OSUpgradeOperation struct {
	Actions []string
}

// NewOSUpgradeOperation creates a new operation with the defined actions
func NewOSUpgradeOperation() *OSUpgradeOperation {
    return &OSUpgradeOperation{
        Actions: []string{
            "Safetycheck",
            "Deviceisolation",
            "Preloadimage",
            "Setbootpartition",
            "Rebootdevice",
        },
    }
}

// Proceed checks if the current action's state is "proceed"
func (op *OSUpgradeOperation) Proceed(device **samplev1alpha1.NetworkDevice) bool {
    // if no operation action is defined, cannot proceed
	deviceStatus := (*device).Status
	if deviceStatus.Operation == nil  {
		return false
	}

	if deviceStatus.Operation.OperationAction == nil {
		return true
	}
	return deviceStatus.Operation.OperationAction.Status == "Proceed"
}

// NextAction returns the next action after the current one
func (op *OSUpgradeOperation) NextAction(device **samplev1alpha1.NetworkDevice) (string, bool) {
    // if no operation action is defined, return the first action
	deviceStatus := (*device).Status
	if deviceStatus.Operation == nil || deviceStatus.Operation.OperationAction == nil {
		return op.Actions[0], true
	}
	
	for i, action := range op.Actions {
		if action == deviceStatus.Operation.OperationAction.Name {
			if i+1 < len(op.Actions) {
				return op.Actions[i+1], true
            }
            return "", false // No next action
        }
    }
    // If current action is not found, return the first action
    return op.Actions[0], true
}
