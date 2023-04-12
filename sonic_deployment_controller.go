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

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
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
	deploymentutil "k8s.io/kubectl/pkg/util/deployment"

	samplev1alpha1 "k8s.io/sample-controller/pkg/apis/sonick8s/v1alpha1"
	clientset "k8s.io/sample-controller/pkg/sonick8s/generated/clientset/versioned"
	samplescheme "k8s.io/sample-controller/pkg/sonick8s/generated/clientset/versioned/scheme"
	informers "k8s.io/sample-controller/pkg/sonick8s/generated/informers/externalversions/sonick8s/v1alpha1"
	listers "k8s.io/sample-controller/pkg/sonick8s/generated/listers/sonick8s/v1alpha1"
)

const sonicControllerAgentName = "sonic-controller"

// Controller is the controller implementation for Foo resources
type SonicDaemonsetDeploymentController struct {
	// kubeclientset is a standard kubernetes clientset
	kubeclientset kubernetes.Interface
	// sampleclientset is a clientset for our own API group
	sampleclientset clientset.Interface

	deploymentsLister appslisters.DaemonSetLister
	deploymentsSynced cache.InformerSynced
	foosLister        listers.SonicDaemonSetDeploymentLister
	foosSynced        cache.InformerSynced

	// workqueue is a rate limited work queue. This is used to queue work to be
	// processed instead of performing it as soon as a change happens. This
	// means we can ensure we only process a fixed amount of resources at a
	// time, and makes it easy to ensure we are never processing the same item
	// simultaneously in two different workers.
	workqueue workqueue.RateLimitingInterface
	// recorder is an event recorder for recording Event resources to the
	// Kubernetes API.
	recorder record.EventRecorder
}

// NewController returns a new sample controller
func NewSonicDaemonsetDeploymentController(
	ctx context.Context,
	kubeclientset kubernetes.Interface,
	sampleclientset clientset.Interface,
	deploymentInformer appsinformers.DaemonSetInformer,
	fooInformer informers.SonicDaemonSetDeploymentInformer) *SonicDaemonsetDeploymentController {
	logger := klog.FromContext(ctx)

	// Create event broadcaster
	// Add sample-controller types to the default Kubernetes Scheme so Events can be
	// logged for sample-controller types.
	utilruntime.Must(samplescheme.AddToScheme(scheme.Scheme))
	logger.V(4).Info("Creating event broadcaster")

	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartStructuredLogging(0)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeclientset.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: controllerAgentName})

	controller := &SonicDaemonsetDeploymentController{
		kubeclientset:     kubeclientset,
		sampleclientset:   sampleclientset,
		deploymentsLister: deploymentInformer.Lister(),
		deploymentsSynced: deploymentInformer.Informer().HasSynced,
		foosLister:        fooInformer.Lister(),
		foosSynced:        fooInformer.Informer().HasSynced,
		workqueue:         workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "Foos"),
		recorder:          recorder,
	}

	logger.Info("Setting up event handlers")
	// Set up an event handler for when Foo resources change
	fooInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.enqueueFoo,
		UpdateFunc: func(old, new interface{}) {
			controller.enqueueFoo(new)
		},
	})
	// Set up an event handler for when Deployment resources change. This
	// handler will lookup the owner of the given Deployment, and if it is
	// owned by a Foo resource then the handler will enqueue that Foo resource for
	// processing. This way, we don't need to implement custom logic for
	// handling Deployment resources. More info on this pattern:
	// https://github.com/kubernetes/community/blob/8cafef897a22026d42f5e5bb3f104febe7e29830/contributors/devel/controllers.md
	deploymentInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.handleObject,
		UpdateFunc: func(old, new interface{}) {
			newDepl := new.(*appsv1.DaemonSet)
			oldDepl := old.(*appsv1.DaemonSet)
			if newDepl.ResourceVersion == oldDepl.ResourceVersion {
				// Periodic resync will send update events for all known Deployments.
				// Two different versions of the same Deployment will always have different RVs.
				return
			}
			controller.handleObject(new)
		},
		DeleteFunc: controller.handleObject,
	})

	return controller
}

// Run will set up the event handlers for types we are interested in, as well
// as syncing informer caches and starting workers. It will block until stopCh
// is closed, at which point it will shutdown the workqueue and wait for
// workers to finish processing their current work items.
func (c *SonicDaemonsetDeploymentController) Run(ctx context.Context, workers int) error {
	defer utilruntime.HandleCrash()
	defer c.workqueue.ShutDown()
	logger := klog.FromContext(ctx)

	// Start the informer factories to begin populating the informer caches
	logger.Info("Starting Foo controller")

	// Wait for the caches to be synced before starting workers
	logger.Info("Waiting for informer caches to sync")

	if ok := cache.WaitForCacheSync(ctx.Done(), c.deploymentsSynced, c.foosSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	logger.Info("Starting workers", "count", workers)
	// Launch two workers to process Foo resources
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
func (c *SonicDaemonsetDeploymentController) runWorker(ctx context.Context) {
	for c.processNextWorkItem(ctx) {
	}
}

// processNextWorkItem will read a single work item off the workqueue and
// attempt to process it, by calling the syncHandler.
func (c *SonicDaemonsetDeploymentController) processNextWorkItem(ctx context.Context) bool {
	obj, shutdown := c.workqueue.Get()
	logger := klog.FromContext(ctx)

	if shutdown {
		return false
	}

	// We wrap this block in a func so we can defer c.workqueue.Done.
	err := func(obj interface{}) error {
		// We call Done here so the workqueue knows we have finished
		// processing this item. We also must remember to call Forget if we
		// do not want this work item being re-queued. For example, we do
		// not call Forget if a transient error occurs, instead the item is
		// put back on the workqueue and attempted again after a back-off
		// period.
		defer c.workqueue.Done(obj)
		var key string
		var ok bool
		// We expect strings to come off the workqueue. These are of the
		// form namespace/name. We do this as the delayed nature of the
		// workqueue means the items in the informer cache may actually be
		// more up to date that when the item was initially put onto the
		// workqueue.
		if key, ok = obj.(string); !ok {
			// As the item in the workqueue is actually invalid, we call
			// Forget here else we'd go into a loop of attempting to
			// process a work item that is invalid.
			c.workqueue.Forget(obj)
			utilruntime.HandleError(fmt.Errorf("expected string in workqueue but got %#v", obj))
			return nil
		}
		// Run the syncHandler, passing it the namespace/name string of the
		// Foo resource to be synced.
		if err := c.syncHandler(ctx, key); err != nil {
			// Put the item back on the workqueue to handle any transient errors.
			c.workqueue.AddRateLimited(key)
			return fmt.Errorf("error syncing '%s': %s, requeuing", key, err.Error())
		}
		// Finally, if no error occurs we Forget this item so it does not
		// get queued again until another change happens.
		c.workqueue.Forget(obj)
		logger.Info("Successfully synced", "resourceName", key)
		return nil
	}(obj)

	if err != nil {
		utilruntime.HandleError(err)
		return true
	}

	return true
}

// syncHandler compares the actual state with the desired, and attempts to
// converge the two. It then updates the Status block of the Foo resource
// with the current status of the resource.
func (c *SonicDaemonsetDeploymentController) syncHandler(ctx context.Context, key string) error {
	// Convert the namespace/name string into a distinct namespace and name
	logger := klog.LoggerWithValues(klog.FromContext(ctx), "resourceName", key)

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		return nil
	}

	// Get the Foo resource with this namespace/name
	foo, err := c.foosLister.SonicDaemonSetDeployments(namespace).Get(name)
	if err != nil {
		// The Foo resource may no longer exist, in which case we stop
		// processing.
		if errors.IsNotFound(err) {
			utilruntime.HandleError(fmt.Errorf("foo '%s' in work queue no longer exists", key))
			return nil
		}

		return err
	}

	deploymentName := foo.Spec.DeploymentName
	if deploymentName == "" {
		// We choose to absorb the error here as the worker would requeue the
		// resource otherwise. Instead, the next time the resource is updated
		// the resource will be queued again.
		utilruntime.HandleError(fmt.Errorf("%s: deployment name must be specified", key))
		return nil
	}

	// Get daemonset list
	dsList, err := c.deploymentsLister.DaemonSets(foo.Namespace).List(labels.Everything())
	// If the resource doesn't exist, we'll create it
	if err != nil {
		logger.V(4).Info("fail to list daemonset")
		return err
	}

	// If an error occurs during Get/Create, we'll requeue the item so we can
	// attempt processing again later. This could have been caused by a
	// temporary network failure, or any other transient reason.

	// If the Deployment is not controlled by this Foo resource, we should log
	// a warning to the event recorder and return error msg.
	/*
		if !metav1.IsControlledBy(deployment, foo) {
			msg := fmt.Sprintf(MessageResourceExists, deployment.Name)
			c.recorder.Event(foo, corev1.EventTypeWarning, ErrResourceExists, msg)
			return fmt.Errorf("%s", msg)
		}

		// If this number of the replicas on the Foo resource is specified, and the
		// number does not equal the current desired replicas on the Deployment, we
		// should update the Deployment resource.
		if foo.Spec.Replicas != nil && *foo.Spec.Replicas != *deployment.Spec.Replicas {
			logger.V(4).Info("Update deployment resource", "currentReplicas", *foo.Spec.Replicas, "desiredReplicas", *deployment.Spec.Replicas)
			deployment, err = c.kubeclientset.AppsV1().Deployments(foo.Namespace).Update(context.TODO(), newDeployment(foo), metav1.UpdateOptions{})
		}
	*/
	// If an error occurs during Update, we'll requeue the item so we can
	// attempt processing again later. This could have been caused by a
	// temporary network failure, or any other transient reason.
	if err != nil {
		return err
	}

	// Finally, we update the status block of the Foo resource to reflect the
	// current state of the world
	err = c.updateFooStatus(foo, dsList)
	if err != nil {
		return err
	}

	c.recorder.Event(foo, corev1.EventTypeNormal, SuccessSynced, MessageResourceSynced)
	return nil
}

func (c *SonicDaemonsetDeploymentController) updateFooStatus(foo *samplev1alpha1.SonicDaemonSetDeployment, deployments []*appsv1.DaemonSet) error {
	// NEVER modify objects from the store. It's a read-only, local cache.
	// You can use DeepCopy() to make a deep copy of original object and modify this copy
	// Or create a copy manually for better performance
	logger := klog.LoggerWithValues(klog.FromContext(context.TODO()), "resourceName", foo.Name)
	fooCopy := foo.DeepCopy()
	fooCopy.Status.DaemonsetList = []samplev1alpha1.DaemonSetItem{}
	//dsMap := make(map[string]string)

	for _, v := range deployments {
		fooCopy.Status.DaemonsetList = append(fooCopy.Status.DaemonsetList, samplev1alpha1.DaemonSetItem{DaemonSetName: v.Name, DaemonSetVersion: v.Spec.Template.Spec.Containers[0].Image})
		/*
			if _, ok := dsMap[v.Name]; !ok {
				updated = true
				logger.Info(fmt.Sprintf("Add ds %s", v.Name))
				dsMap[v.Name] = v.Spec.Template.Spec.Containers[0].Image
				fooCopy.Status.DaemonsetList = append(fooCopy.Status.DaemonsetList, samplev1alpha1.DaemonSetItem{DaemonSetName: v.Name, DaemonSetVersion: v.Spec.Template.Spec.Containers[0].Image})
			} else {
				if dsMap[v.Name] != v.Spec.Template.Spec.Containers[0].Image {
					for _, d := range fooCopy.Status.DaemonsetList {
						if d.DaemonSetName == v.Name {
							d.DaemonSetVersion = dsMap[v.Name]
						}
					}
				}
			}
		*/
		// update daemonset if version is mismatch

	}
	_, err := c.sampleclientset.SonicV1alpha1().SonicDaemonSetDeployments(foo.Namespace).UpdateStatus(context.TODO(), fooCopy, metav1.UpdateOptions{})
	//rolling update for daemonset
	for _, v := range deployments {
		if foo.Spec.DaemonSetVersion != v.Spec.Template.Spec.Containers[0].Image {
			logger.Info(fmt.Sprintf("Need to update ds versoin for ds %s to %s", v.Name, foo.Spec.DaemonSetVersion))
			c.updateDaemonset(v, foo.Spec.DaemonSetVersion)
			time.Sleep(10 * time.Second)
			break
		}
	}
	/*
		if !updated {
			return nil
		}
	*/
	// If the CustomResourceSubresources feature gate is not enabled,
	// we must use Update instead of UpdateStatus to update the Status block of the Foo resource.
	// UpdateStatus will not allow changes to the Spec of the resource,
	// which is ideal for ensuring nothing other than resource status has been updated.

	return err
}

// enqueueFoo takes a Foo resource and converts it into a namespace/name
// string which is then put onto the work queue. This method should *not* be
// passed resources of any type other than Foo.
func (c *SonicDaemonsetDeploymentController) enqueueFoo(obj interface{}) {
	var key string
	var err error
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.workqueue.Add(key)
}

// handleObject will take any resource implementing metav1.Object and attempt
// to find the Foo resource that 'owns' it. It does this by looking at the
// objects metadata.ownerReferences field for an appropriate OwnerReference.
// It then enqueues that Foo resource to be processed. If the object does not
// have an appropriate OwnerReference, it will simply be skipped.
func (c *SonicDaemonsetDeploymentController) handleObject(obj interface{}) {
	var object metav1.Object
	var ok bool
	logger := klog.FromContext(context.Background())
	if object, ok = obj.(metav1.Object); !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("error decoding object, invalid type"))
			return
		}
		object, ok = tombstone.Obj.(metav1.Object)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("error decoding object tombstone, invalid type"))
			return
		}
		logger.V(4).Info("Recovered deleted object", "resourceName", object.GetName())
	}
	logger.V(4).Info("Processing object", "object", klog.KObj(object))
	if ownerRef := metav1.GetControllerOf(object); ownerRef != nil {
		// If this object is not owned by a Foo, we should not do anything more
		// with it.
		if ownerRef.Kind == "" {
			return
		}

		foo, err := c.foosLister.SonicDaemonSetDeployments(object.GetNamespace()).Get("dc-ds-demo")
		if err != nil {
			logger.V(4).Info("Ignore orphaned object", "object", klog.KObj(object), "foo", ownerRef.Name)
			return
		}
		logger.V(4).Info("enque the dc-ds-demo")
		c.enqueueFoo(foo)
		return
	}
}

func (c *SonicDaemonsetDeploymentController) getDeploymentPatch(podTemplate *corev1.PodTemplateSpec, annotations map[string]string) (types.PatchType, []byte, error) {
	// Create a patch of the Deployment that replaces spec.template
	patch, err := json.Marshal([]interface{}{
		map[string]interface{}{
			"op":    "replace",
			"path":  "/spec/template",
			"value": podTemplate,
		},
		map[string]interface{}{
			"op":    "replace",
			"path":  "/metadata/annotations",
			"value": annotations,
		},
	})
	return types.JSONPatchType, patch, err
}

func (c *SonicDaemonsetDeploymentController) updateDaemonset(ds *appsv1.DaemonSet, version string) (bool, error) {
	klog.Infof("Aboute to update daemonset %s/%s to %s", ds.Namespace, ds, version)
	targetDs := ds.DeepCopy()
	var annotationsToSkip = map[string]bool{
		corev1.LastAppliedConfigAnnotation:       true,
		deploymentutil.RevisionAnnotation:        true,
		deploymentutil.RevisionHistoryAnnotation: true,
		deploymentutil.DesiredReplicasAnnotation: true,
		deploymentutil.MaxReplicasAnnotation:     true,
		appsv1.DeprecatedRollbackTo:              true,
	}
	delete(targetDs.Spec.Template.Labels, appsv1.DefaultDeploymentUniqueLabelKey)

	// compute deployment annotations
	annotations := map[string]string{}
	for k := range annotationsToSkip {
		if v, ok := ds.Annotations[k]; ok {
			annotations[k] = v
		}
	}
	for k, v := range targetDs.Annotations {
		if !annotationsToSkip[k] {
			annotations[k] = v
		}
	}

	targetDs.Spec.Template.Spec.Containers[0].Image = version
	patchType, patch, err := c.getDeploymentPatch(&targetDs.Spec.Template, targetDs.Annotations)
	if err != nil {
		return false, fmt.Errorf("failed restoring revision %s: %v", targetDs.ResourceVersion, err)
	}

	patchOptions := metav1.PatchOptions{}

	// update ds revision
	if _, err = c.kubeclientset.AppsV1().DaemonSets(targetDs.Namespace).Patch(context.TODO(), targetDs.Name, patchType, patch, patchOptions); err != nil {
		klog.Errorf("Updating daemonset %s/%s is failed:%v", targetDs.Namespace, targetDs.Name, err)
		return false, fmt.Errorf("failed restoring revision %s: %v", targetDs.ResourceVersion, err)
	}
	klog.Infof("Updating daemonset %s/%s is successful", targetDs.Namespace, targetDs.Name)
	return true, nil
}

// newDeployment creates a new Deployment for a Foo resource. It also sets
// the appropriate OwnerReferences on the resource so handleObject can discover
// the Foo resource that 'owns' it.
