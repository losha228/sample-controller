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

package controller

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/time/rate"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	appsinformers "k8s.io/client-go/informers/apps/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	appslisters "k8s.io/client-go/listers/apps/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"

	networkdevicev1 "k8s.io/sample-controller/pkg/apis/networkdevice/v1"
    opt "k8s.io/sample-controller/pkg/networkoperation"
	clientset "k8s.io/sample-controller/pkg/networkdevice/generated/clientset/versioned"
	samplescheme "k8s.io/sample-controller/pkg/networkdevice/generated/clientset/versioned/scheme"
	informers "k8s.io/sample-controller/pkg/networkdevice/generated/informers/externalversions/networkdevice/v1"
	listers "k8s.io/sample-controller/pkg/networkdevice/generated/listers/networkdevice/v1"
)

const controllerAgentName = "devicelifecycle-controller"

const (
	// SuccessSynced is used as part of the Event 'reason' when a NetworkDevice is synced
	SuccessSynced = "Synced"
	// ErrResourceExists is used as part of the Event 'reason' when a NetworkDevice fails
	// to sync due to a Deployment of the same name already existing.
	ErrResourceExists = "ErrResourceExists"

	// MessageResourceExists is the message used for Events when a resource
	// fails to sync due to a Deployment already existing
	MessageResourceExists = "Resource %q already exists and is not managed by NetworkDevice"
	// MessageResourceSynced is the message used for an Event fired when a NetworkDevice
	// is synced successfully
	MessageResourceSynced = "NetworkDevice synced successfully"
	// FieldManager distinguishes this controller from other things writing to API objects
	FieldManager = controllerAgentName
)

// DeviceLifecycleController is the controller implementation for NetworkDevice resources
type DeviceLifecycleController struct {
	// kubeclientset is a standard kubernetes clientset
	kubeclientset kubernetes.Interface
	// sampleclientset is a clientset for our own API group
	sampleclientset clientset.Interface

	deploymentsLister appslisters.DeploymentLister
	deploymentsSynced cache.InformerSynced
	networkDeviceLister        listers.NetworkDeviceLister
	networkDevicesSynced        cache.InformerSynced

	// workqueue is a rate limited work queue. This is used to queue work to be
	// processed instead of performing it as soon as a change happens. This
	// means we can ensure we only process a fixed amount of resources at a
	// time, and makes it easy to ensure we are never processing the same item
	// simultaneously in two different workers.
	workqueue workqueue.TypedRateLimitingInterface[cache.ObjectName]
	// recorder is an event recorder for recording Event resources to the
	// Kubernetes API.
	recorder record.EventRecorder

	// operation map
	operationHandlers map[string]opt.OperationHandler
}

// NewController returns a new sample controller
func NewDeviceLifecycleController(
	ctx context.Context,
	kubeclientset kubernetes.Interface,
	sampleclientset clientset.Interface,
	deploymentInformer appsinformers.DeploymentInformer,
	networkDeviceInformer informers.NetworkDeviceInformer) *DeviceLifecycleController {
	logger := klog.FromContext(ctx)

	// Create event broadcaster
	// Add sample-controller types to the default Kubernetes Scheme so Events can be
	// logged for sample-controller types.
	utilruntime.Must(samplescheme.AddToScheme(scheme.Scheme))
	logger.V(4).Info("Creating event broadcaster")

	eventBroadcaster := record.NewBroadcaster(record.WithContext(ctx))
	eventBroadcaster.StartStructuredLogging(0)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeclientset.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: controllerAgentName})
	ratelimiter := workqueue.NewTypedMaxOfRateLimiter(
		workqueue.NewTypedItemExponentialFailureRateLimiter[cache.ObjectName](5*time.Millisecond, 1000*time.Second),
		&workqueue.TypedBucketRateLimiter[cache.ObjectName]{Limiter: rate.NewLimiter(rate.Limit(50), 300)},
	)

	controller := &DeviceLifecycleController{
		kubeclientset:     kubeclientset,
		sampleclientset:   sampleclientset,
		deploymentsLister: deploymentInformer.Lister(),
		deploymentsSynced: deploymentInformer.Informer().HasSynced,
		networkDeviceLister:        networkDeviceInformer.Lister(),
		networkDevicesSynced:        networkDeviceInformer.Informer().HasSynced,
		workqueue:         workqueue.NewTypedRateLimitingQueue(ratelimiter),
		recorder:          recorder,
	}

	logger.Info("Setting up event handlers")
	// Set up an event handler for when Device resources change
	networkDeviceInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.enqueueDevice,
		UpdateFunc: func(old, new interface{}) {
			controller.enqueueDevice(new)
		},
	})
	
	return controller
}

// Run will set up the event handlers for types we are interested in, as well
// as syncing informer caches and starting workers. It will block until stopCh
// is closed, at which point it will shutdown the workqueue and wait for
// workers to finish processing their current work items.
func (c *DeviceLifecycleController) Run(ctx context.Context, workers int) error {
	defer utilruntime.HandleCrash()
	defer c.workqueue.ShutDown()
	logger := klog.FromContext(ctx)

	// Start the informer factories to begin populating the informer caches
	logger.Info("Starting NetworkDevice controller")

	// Wait for the caches to be synced before starting workers
	logger.Info("Waiting for informer caches to sync")

	if ok := cache.WaitForCacheSync(ctx.Done(), c.deploymentsSynced, c.networkDevicesSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	logger.Info("Starting workers", "count", workers)
	// Launch two workers to process NetworkDevice resources
	for i := 0; i < workers; i++ {
		go wait.UntilWithContext(ctx, c.runWorker, time.Second)
	}

	logger.Info("Started workers")
	<-ctx.Done()
	logger.Info("Shutting down workers")

	return nil
}

// runWorker is a long-running function that will continually call the
// processNextWorkItem function in order to read and process a message on the
// workqueue.
func (c *DeviceLifecycleController) runWorker(ctx context.Context) {
	for c.processNextWorkItem(ctx) {
	}
}

// processNextWorkItem will read a single work item off the workqueue and
// attempt to process it, by calling the syncHandler.
func (c *DeviceLifecycleController) processNextWorkItem(ctx context.Context) bool {
	objRef, shutdown := c.workqueue.Get()
	logger := klog.FromContext(ctx)

	if shutdown {
		return false
	}

	// We call Done at the end of this func so the workqueue knows we have
	// finished processing this item. We also must remember to call Forget
	// if we do not want this work item being re-queued. For example, we do
	// not call Forget if a transient error occurs, instead the item is
	// put back on the workqueue and attempted again after a back-off
	// period.
	defer c.workqueue.Done(objRef)

	// Run the syncHandler, passing it the structured reference to the object to be synced.
	err := c.syncHandler(ctx, objRef)
	if err == nil {
		// If no error occurs then we Forget this item so it does not
		// get queued again until another change happens.
		c.workqueue.Forget(objRef)
		logger.Info("Successfully synced", "objectName", objRef)
		return true
	}
	// there was a failure so be sure to report it.  This method allows for
	// pluggable error handling which can be used for things like
	// cluster-monitoring.
	utilruntime.HandleErrorWithContext(ctx, err, "Error syncing; requeuing for later retry", "objectReference", objRef)
	// since we failed, we should requeue the item to work on later.  This
	// method will add a backoff to avoid hotlooping on particular items
	// (they're probably still not going to work right away) and overall
	// controller protection (everything I've done is broken, this controller
	// needs to calm down or it can starve other useful work) cases.
	c.workqueue.AddRateLimited(objRef)
	return true
}

// syncHandler compares the actual state with the desired, and attempts to
// converge the two. It then updates the Status block of the NetworkDevice resource
// with the current status of the resource.
func (c *DeviceLifecycleController) syncHandler(ctx context.Context, objectRef cache.ObjectName) error {
	logger := klog.LoggerWithValues(klog.FromContext(ctx), "objectRef", objectRef)

	// Get the NetworkDevice resource with this namespace/name
	device, err := c.networkDeviceLister.NetworkDevices(objectRef.Namespace).Get(objectRef.Name)
	if err != nil {
		// The NetworkDevice resource may no longer exist, in which case we stop
		// processing.
		if errors.IsNotFound(err) {
			utilruntime.HandleErrorWithContext(ctx, err, "NetworkDevice referenced by item in work queue no longer exists", "objectReference", objectRef)
			return nil
		}

		return err
	}

	// if the status is nil, initialize it
	if device.Status.State == "" {
		device.Status = networkdevicev1.NetworkDeviceStatus{
			State: string(networkdevicev1.NetworkDeviceStateHealthy),
		}
		c.updateNetworkDeviceStatus(ctx, device)
		logger.Info("Initialized NetworkDevice status", "deviceStatus", device.Status)
		return nil
	}

	// if device has no status or no operation, nothing to do
	if device.Spec.Operation == "" {
		logger.Info("NetworkDevice has no operation defined, skipping processing")
		return nil
	}

    // only handle OSUpgradeOperation operations for demo purposes
	if device.Spec.Operation != "OSUpgradeOperation" {
		logger.Info("Ignoring NetworkDevice with unsupported operation type", "operationType", device.Spec.Operation)
		return nil
	}

	operationHandler := opt.NewOSUpgradeOperation()
	
	if !operationHandler.Proceed(device) {
		logger.Info("Operation action state is not 'Proceed', skipping processing",  "status", device.Status)
		return nil
	}

	// move to the next action
	nextAction, hasNext := operationHandler.NextAction(device)
	if hasNext {
		logger.Info("Moving to next action", "nextAction", nextAction)
		device.Spec.OperationAction = nextAction
		device.Status.OperationActionState = "Pending"

	} else {
		logger.Info("No more actions left, marking operation as Completed")
		device.Status.OperationState = "Completed"
	}

	c.updateNetworkDeviceStatus(ctx, device)
	// just logging for demo purposes
	logger.Info("Processing NetworkDevice", "deviceSpec", device.Spec, "deviceStatus", device.Status)

	c.recorder.Event(device, corev1.EventTypeNormal, SuccessSynced, MessageResourceSynced)
	return nil
}

func (c *DeviceLifecycleController) updateNetworkDeviceStatus(ctx context.Context, device *networkdevicev1.NetworkDevice) error {
	// NEVER modify objects from the store. It's a read-only, local cache.
	// You can use DeepCopy() to make a deep copy of original object and modify this copy
	// Or create a copy manually for better performance
	deviceCopy := device.DeepCopy()
	deviceCopy.Status.State = "Healthy"
	if deviceCopy.Status.LastTransitionTime == (metav1.Time{}) {
		deviceCopy.Status.LastTransitionTime = metav1.Now()
	}

	// set the os info in status to match spec for demo purposes
	deviceCopy.Status.OsVersion = deviceCopy.Spec.OsVersion

	// logging the status update for demo purposes
	logger := klog.FromContext(ctx)
	msg := fmt.Sprintf("Updating NetworkDevice %s status to %v", deviceCopy.Name, deviceCopy.Status.OperationState)
	logger.Info(msg)
	
	_, err := c.sampleclientset.Sonick8sV1().NetworkDevices(device.Namespace).UpdateStatus(ctx, deviceCopy, metav1.UpdateOptions{FieldManager: FieldManager})
	return err
}

// enqueueDevice takes a Device resource and converts it into a namespace/name
// string which is then put onto the work queue. This method should *not* be
// passed resources of any type other than Device.
func (c *DeviceLifecycleController) enqueueDevice(obj interface{}) {
	if objectRef, err := cache.ObjectToName(obj); err != nil {
		utilruntime.HandleError(err)
		return
	} else {
		c.workqueue.Add(objectRef)
	}
}

// handleObject will take any resource implementing metav1.Object and attempt
// to find the Device resource that 'owns' it. It does this by looking at the
// objects metadata.ownerReferences field for an appropriate OwnerReference.
// It then enqueues that Device resource to be processed. If the object does not
// have an appropriate OwnerReference, it will simply be skipped.
func (c *DeviceLifecycleController) handleObject(obj interface{}) {
	var object metav1.Object
	var ok bool
	logger := klog.FromContext(context.Background())
	if object, ok = obj.(metav1.Object); !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			// If the object value is not too big and does not contain sensitive information then
			// it may be useful to include it.
			utilruntime.HandleErrorWithContext(context.Background(), nil, "Error decoding object, invalid type", "type", fmt.Sprintf("%T", obj))
			return
		}
		object, ok = tombstone.Obj.(metav1.Object)
		if !ok {
			// If the object value is not too big and does not contain sensitive information then
			// it may be useful to include it.
			utilruntime.HandleErrorWithContext(context.Background(), nil, "Error decoding object tombstone, invalid type", "type", fmt.Sprintf("%T", tombstone.Obj))
			return
		}
		logger.V(4).Info("Recovered deleted object", "resourceName", object.GetName())
	}
	logger.V(4).Info("Processing object", "object", klog.KObj(object))
	if ownerRef := metav1.GetControllerOf(object); ownerRef != nil {
		// If this object is not owned by a NetworkDevice, we should not do anything more
		// with it.
		if ownerRef.Kind != "NetworkDevice" {
			return
		}

		networkDevice, err := c.networkDeviceLister.NetworkDevices(object.GetNamespace()).Get(ownerRef.Name)
		if err != nil {
			logger.V(4).Info("Ignore orphaned object", "object", klog.KObj(networkDevice), "networkDevice", ownerRef.Name)
			return
		}

		c.enqueueDevice(networkDevice)
		return
	}
}
