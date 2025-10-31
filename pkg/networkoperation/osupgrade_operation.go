package networkoperation

import (
	networkdevicev1 "k8s.io/sample-controller/pkg/apis/networkdevice/v1"
)


type OSUpgradeOperation struct {
	Actions []string
}

// NewOSUpgradeOperation creates a new operation with the defined actions
func NewOSUpgradeOperation() *OSUpgradeOperation {
    return &OSUpgradeOperation{
        Actions: []string{
            "SafetyCheck",
            "DeviceIsolation",
            "PreloadImage",
            "SetBootPartition",
            "RebootDevice",
        },
    }
}

// Proceed checks if the current action's state is "proceed"
func (op *OSUpgradeOperation) Proceed(device *networkdevicev1.NetworkDevice) bool {
    	if device.Spec.Operation != OperationOSUpgrade {
		return false
	}

	return device.Status.OperationActionState == string(OperationActionStateProceed)
}

// NextAction returns the next action after the current one
func (op *OSUpgradeOperation) NextAction(device *networkdevicev1.NetworkDevice) (string, bool) {
    if device.Spec.Operation != OperationOSUpgrade {
		return "", false
	}
	// if no operation action is defined, return the first action
	if  device.Spec.OperationAction == "" {
		return op.Actions[0], true
	}
	
	for i, action := range op.Actions {
		if action == device.Spec.OperationAction {
			if i+1 < len(op.Actions) {
				return op.Actions[i+1], true
            }
            return "", false // No next action
        }
    }
    // If current action is not found, return the first action
    return op.Actions[0], true
}
