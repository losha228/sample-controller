package networkoperation

import (
	networkdevicev1 "k8s.io/sample-controller/pkg/apis/networkdevice/v1"
)

type OperationActionState string
type OperationState string

const (
	// operation names
	OperationOSUpgrade    = "OSUpgrade"
	OperationPreloadImage = "PreloadImage"

	// operation action states
	OperationActionStatePending    OperationActionState = "Pending"
	OperationActionStateInProgress OperationActionState = "InProgress"
	OperationActionStateProceed    OperationActionState = "Proceed"
	OperationActionStateFailed     OperationActionState = "Failed"

	// operation states
	OperationStatePending    OperationState = "Pending"
	OperationStateProceed    OperationState = "Proceed"
	OperationStateSucceeded  OperationState = "Succeeded"
	OperationStateFailed     OperationState = "Failed"
)

// define an interface for operation handling
type OperationHandler interface {
	Proceed(device *networkdevicev1.NetworkDevice) bool
	NextAction(device *networkdevicev1.NetworkDevice) (string, bool)
}

type PreloadImageOperation struct {
	Actions []string
}

// NewPreloadImageOperation creates a new operation with the defined actions
func NewPreloadImageOperation() *PreloadImageOperation {
    return &PreloadImageOperation{
        Actions: []string{
            "PreloadImage",
        },
    }
}

// Proceed checks if the current action's state is "proceed"
func (op *PreloadImageOperation) Proceed(device *networkdevicev1.NetworkDevice) bool {
    // if no operation action is defined, cannot proceed
	deviceStatus := device.Status
	if device.Spec.Operation != OperationPreloadImage {
		return false
	}

	return deviceStatus.OperationActionState == string(OperationActionStateProceed)
}

// NextAction returns the next action after the current one
func (op *PreloadImageOperation) NextAction(device *networkdevicev1.NetworkDevice) (string, bool) {
    	if device.Spec.Operation != OperationPreloadImage {
		return "", false
	}

	// always return the only action for PreloadImage operation
    return op.Actions[0], true
}
