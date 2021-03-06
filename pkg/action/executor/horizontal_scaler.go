package executor

import (
	"errors"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	client "k8s.io/client-go/kubernetes"
	api "k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/pkg/apis/apps/v1beta1"

	"github.com/turbonomic/kubeturbo/pkg/action/turboaction"
	"github.com/turbonomic/kubeturbo/pkg/action/util"
	discutil "github.com/turbonomic/kubeturbo/pkg/discovery/util"
	turboscheduler "github.com/turbonomic/kubeturbo/pkg/scheduler"
	"github.com/turbonomic/kubeturbo/pkg/turbostore"

	"github.com/turbonomic/turbo-go-sdk/pkg/proto"

	"github.com/golang/glog"
)

var (
	getOption = metav1.GetOptions{}
)

type HorizontalScaler struct {
	kubeClient *client.Clientset
	broker     turbostore.Broker
	scheduler  *turboscheduler.TurboScheduler
}

func NewHorizontalScaler(client *client.Clientset, broker turbostore.Broker,
	scheduler *turboscheduler.TurboScheduler) *HorizontalScaler {
	return &HorizontalScaler{
		kubeClient: client,
		broker:     broker,
		scheduler:  scheduler,
	}
}

func (h *HorizontalScaler) Execute(actionItem *proto.ActionItemDTO) (*turboaction.TurboAction, error) {
	if actionItem == nil {
		return nil, errors.New("ActionItem passed in is nil")
	}
	action, err := h.buildPendingScalingTurboAction(actionItem)
	if err != nil {
		return nil, err
	}

	return h.horizontalScale(action)
}

func (h *HorizontalScaler) buildPendingScalingTurboAction(actionItem *proto.ActionItemDTO) (*turboaction.TurboAction,
	error) {
	targetSE := actionItem.GetTargetSE()
	targetEntityType := targetSE.GetEntityType()
	if targetEntityType != proto.EntityDTO_CONTAINER_POD && targetEntityType != proto.EntityDTO_APPLICATION {
		return nil, errors.New("The target service entity for scaling action is " +
			"neither a Pod nor an Application.")
	}

	providerPod, err := h.getProviderPod(actionItem)
	if err != nil {
		return nil, fmt.Errorf("Try to scaling %s, but cannot find a pod related to it in the cluster: %s",
			targetSE.GetId(), err)
	}
	glog.V(3).Infof("Got the provider pod %s/%s", providerPod.Namespace, providerPod.Name)

	targetObject := &turboaction.TargetObject{
		TargetObjectUID:       string(providerPod.UID),
		TargetObjectNamespace: providerPod.Namespace,
		TargetObjectName:      providerPod.Name,
		TargetObjectType:      turboaction.TypePod,
	}

	var parentObjRef *turboaction.ParentObjectRef
	parentRefObject, _ := discutil.FindParentReferenceObject(providerPod)
	if parentRefObject != nil {
		parentObjRef = &turboaction.ParentObjectRef{
			ParentObjectUID:       string(parentRefObject.UID),
			ParentObjectNamespace: parentRefObject.Namespace,
			ParentObjectName:      parentRefObject.Name,
			ParentObjectType:      parentRefObject.Kind,
		}
	} else {
		return nil, errors.New("Cannot perform auto-scale, please make sure the pod is connected to " +
			"a replication controller or replica set.")
	}

	// Get diff and action type according scale in or scale out.
	var diff int32
	var actionType turboaction.TurboActionType
	if actionItem.GetActionType() == proto.ActionItemDTO_PROVISION {
		// Scale out, increase the replica. diff = 1.
		diff = 1
		actionType = turboaction.ActionProvision
	} else if actionItem.GetActionType() == proto.ActionItemDTO_MOVE {
		// TODO, unbind action is send as MOVE. This requires server side change.
		// Scale in, decrease the replica. diff = -1.
		diff = -1
		actionType = turboaction.ActionUnbind
	} else {
		return nil, errors.New("Not a scaling action.")
	}

	var scaleSpec turboaction.ScaleSpec
	switch parentRefObject.Kind {
	case turboaction.TypeReplicationController:
		rc, err := util.FindReplicationControllerForPod(h.kubeClient, providerPod)
		if err != nil {
			return nil, fmt.Errorf("Failed to find replication controller for finishing the scaling "+
				"action: %s", err)
		}
		scaleSpec = turboaction.ScaleSpec{
			OriginalReplicas: *rc.Spec.Replicas,
			NewReplicas:      *rc.Spec.Replicas + diff,
		}
		break

	case turboaction.TypeReplicaSet:
		deployment, err := util.FindDeploymentForPod(h.kubeClient, providerPod)
		if err != nil {
			return nil, fmt.Errorf("Failed to find deployment for finishing the scaling "+
				"action: %s", err)
		}
		scaleSpec = turboaction.ScaleSpec{
			OriginalReplicas: *deployment.Spec.Replicas,
			NewReplicas:      *deployment.Spec.Replicas + diff,
		}
		break

	default:
		return nil, fmt.Errorf("Error Scale Pod for %s-%s: Not Supported.",
			parentObjRef.ParentObjectType, parentObjRef.ParentObjectName)
	}

	// Invalid new replica.
	if scaleSpec.NewReplicas < 0 {
		return nil, fmt.Errorf("Invalid new replica %d for %s/%s", scaleSpec.NewReplicas,
			parentRefObject.Namespace, parentRefObject.Name)
	}

	content := turboaction.NewTurboActionContentBuilder(actionType, targetObject).
		ActionSpec(scaleSpec).
		ParentObjectRef(parentObjRef).
		Build()
	action := turboaction.NewTurboActionBuilder(parentObjRef.ParentObjectNamespace, *actionItem.Uuid).
		Content(content).
		Create()
	glog.V(4).Infof("Horizontal scaling action is built as %v", action)

	return &action, nil
}

func (h *HorizontalScaler) getProviderPod(actionItem *proto.ActionItemDTO) (*api.Pod, error) {
	targetEntityType := actionItem.GetTargetSE().GetEntityType()
	var providerPod *api.Pod
	if targetEntityType == proto.EntityDTO_CONTAINER_POD {
		targetPod := actionItem.GetTargetSE()

		// TODO, as there is issue in server, find pod based on entity properties is not supported right now. Once the issue in server gets resolved, we should use the following code to find the pod.
		//podProperties := targetPod.GetEntityProperties()
		//foundPod, err:= util.GetPodFromProperties(h.kubeClient, targetEntityType, podProperties)

		// TODO the following is a temporary fix.
		foundPod, err := util.GetPodFromUUID(h.kubeClient, targetPod.GetId())
		if err != nil {
			return nil, fmt.Errorf("failed to find pod %s in Kubernetes: %s", targetPod.GetDisplayName(),
				err)
		}
		providerPod = foundPod
	} else if targetEntityType == proto.EntityDTO_APPLICATION {
		// TODO, as there is issue in server, find pod based on entity properties is not supported right now. Once the issue in server gets resolved, we should use the following code to find the pod.
		// As we store hosting pod information inside entity properties of an application entity, so we can get
		// the application provider directly from there.
		//appProperties := actionItem.GetTargetSE().GetEntityProperties()
		//foundPod, err:= util.GetPodFromProperties(h.kubeClient, targetEntityType, appProperties)

		// TODO the following is a temporary fix.
		providers := actionItem.GetProviders()
		foundPod, err := util.FindApplicationPodProvider(h.kubeClient, providers)
		if err != nil {
			return nil, fmt.Errorf("failed to find provider pod for %s in Kubernetes: %s",
				actionItem.GetTargetSE().GetDisplayName(), err)
		}
		providerPod = foundPod
	} else if targetEntityType == proto.EntityDTO_VIRTUAL_APPLICATION {
		// Get the current application provider.
		currentSE := actionItem.GetCurrentSE()
		currentEntityType := currentSE.GetEntityType()
		if currentEntityType != proto.EntityDTO_APPLICATION {
			return nil, fmt.Errorf("Unexpected entity type for a Unbind action: %s", currentEntityType)
		}

		// TODO, as there is issue in server, find pod based on entity properties is not supported right now. Once the issue in server resolve, we should use the following code to find the pod.
		// As we store hosting pod information inside entity properties of an application entity, so we can get
		// the application provider directly from there.
		//appProperties := currentSE.GetEntityProperties()
		//foundPod, err:= util.GetPodFromProperties(h.kubeClient, targetEntityType, appProperties)

		// TODO the following is a temporary fix.
		// Here we need to find the ID of the provider pod from commodity bought of application.
		commoditiesBought := currentSE.GetCommoditiesBought()
		var podID string
		for _, cb := range commoditiesBought {
			if cb.GetProviderType() == proto.EntityDTO_CONTAINER_POD {
				podID = cb.GetProviderId()
			}
		}
		if podID == "" {
			return nil, fmt.Errorf("cannot find provider pod for application %s based on commodities "+
				"bought map %++v", currentSE.GetDisplayName(), commoditiesBought)
		}
		foundPod, err := util.GetPodFromUUID(h.kubeClient, podID)
		if err != nil {
			return nil, fmt.Errorf("failed to find pod with ID %s in Kubernetes: %s", podID, err)
		}
		providerPod = foundPod
	}
	return providerPod, nil
}

func (h *HorizontalScaler) horizontalScale(action *turboaction.TurboAction) (*turboaction.TurboAction, error) {
	// 1. Setup consumer
	actionContent := action.Content
	scaleSpec, ok := actionContent.ActionSpec.(turboaction.ScaleSpec)
	if !ok || scaleSpec.NewReplicas < 0 {
		return nil, errors.New("Failed to setup horizontal scaler as the provided scale spec is invalid.")
	}

	var key string
	if actionContent.ParentObjectRef.ParentObjectUID != "" {
		key = actionContent.ParentObjectRef.ParentObjectUID
	} else {
		return nil, errors.New("Failed to setup horizontal scaler consumer: failed to retrieve the UID of " +
			"replication controller or replica set.")
	}
	glog.V(3).Infof("The current horizontal scaler consumer is listening on pod created by replication "+
		"controller or replica set with key %s", key)
	podConsumer := turbostore.NewPodConsumer(string(action.UID), key, h.broker)

	// 2. scale up and down by changing the replica of replication controller or deployment.
	err := h.updateReplica(actionContent.TargetObject, actionContent.ParentObjectRef, actionContent.ActionSpec)
	if err != nil {
		return nil, fmt.Errorf("Failed to update replica: %s", err)
	}

	// 3. If this is an unbind action, it means it is an action with only one stage.
	// So after changing the replica it can return immediately.
	if action.Content.ActionType == turboaction.ActionUnbind {
		// Update turbo action.
		action.Status = turboaction.Executed
		return action, nil
	}

	// 4. Wait for desired pending pod
	// Set a timeout for 5 minutes.
	t := time.NewTimer(secondPhaseTimeoutLimit)
	for {
		select {
		case pod, ok := <-podConsumer.WaitPod():
			if !ok {
				return nil, errors.New("Failed to receive the pending pod generated as a result of " +
					"auto scaling.")
			}
			podConsumer.Leave(key, h.broker)

			// 5. Schedule the pod.
			// TODO: we don't have a destination to provision a pod yet. So here we need to call scheduler. Or we can post back the pod to be scheduled
			err = h.scheduler.Schedule(pod)
			if err != nil {
				return nil, fmt.Errorf("Error scheduling the new provisioned pod: %s", err)
			}

			// 6. Update turbo action.
			action.Status = turboaction.Executed

			return action, nil

		case <-t.C:
			// timeout
			return nil, errors.New("Timed out at the second phase when try to finish the horizontal scale" +
				" process")
		}
	}

}

func (h *HorizontalScaler) updateReplica(targetObject turboaction.TargetObject, parentObjRef turboaction.ParentObjectRef,
	actionSpec turboaction.ActionSpec) error {
	scaleSpec, ok := actionSpec.(turboaction.ScaleSpec)
	if !ok {
		return fmt.Errorf("%++v is not a scale spec", actionSpec)
	}
	providerType := parentObjRef.ParentObjectType
	switch providerType {
	case turboaction.TypeReplicationController:
		rc, err := h.kubeClient.CoreV1().ReplicationControllers(parentObjRef.ParentObjectNamespace).
			Get(parentObjRef.ParentObjectName, getOption)
		if err != nil {
			return fmt.Errorf("Failed to find replication controller for finishing the scaling "+
				"action: %s", err)
		}
		return h.updateReplicationControllerReplicas(rc, scaleSpec.NewReplicas)

	case turboaction.TypeReplicaSet:
		providerPod, err := h.kubeClient.CoreV1().Pods(targetObject.TargetObjectNamespace).Get(targetObject.TargetObjectName, getOption)
		if err != nil {
			return fmt.Errorf("Failed to find deployemnet for finishing the scaling action: %s", err)
		}
		// TODO, here we only support ReplicaSet created by Deployment.
		deployment, err := util.FindDeploymentForPod(h.kubeClient, providerPod)
		if err != nil {
			return fmt.Errorf("Failed to find deployment for finishing the scaling action: %s", err)
		}
		return h.updateDeploymentReplicas(deployment, scaleSpec.NewReplicas)
		break
	default:
		return fmt.Errorf("Unsupported provider type %s", providerType)
	}
	return nil
}

func (h *HorizontalScaler) updateReplicationControllerReplicas(rc *api.ReplicationController, newReplicas int32) error {
	rc.Spec.Replicas = &newReplicas
	namespace := rc.Namespace
	newRC, err := h.kubeClient.CoreV1().ReplicationControllers(namespace).Update(rc)
	if err != nil {
		return fmt.Errorf("Error updating replication controller %s/%s: %s", rc.Namespace, rc.Name, err)
	}
	glog.V(4).Infof("New replicas of %s/%s is %d", newRC.Namespace, newRC.Name, newRC.Spec.Replicas)
	return nil
}

func (h *HorizontalScaler) updateDeploymentReplicas(deployment *v1beta1.Deployment, newReplicas int32) error {
	deployment.Spec.Replicas = &newReplicas
	namespace := deployment.Namespace
	newDeployment, err := h.kubeClient.AppsV1beta1().Deployments(namespace).Update(deployment)
	if err != nil {
		return fmt.Errorf("Error updating replication controller %s/%s: %s",
			deployment.Namespace, deployment.Name, err)
	}
	glog.V(4).Infof("New replicas of %s/%s is %v", newDeployment.Namespace, newDeployment.Name,
		newDeployment.Spec.Replicas)
	return nil
}
