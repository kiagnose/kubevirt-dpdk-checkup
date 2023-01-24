/*
 * This file is part of the kiagnose project
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 * Copyright 2023 Red Hat, Inc.
 *
 */

package checkup

import (
	"context"
	"fmt"
	"log"
	"time"

	k8scorev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	k8srand "k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/util/wait"

	networkv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"

	kvcorev1 "kubevirt.io/api/core/v1"

	"github.com/kiagnose/kubevirt-dpdk-checkup/pkg/internal/checkup/pod"
	"github.com/kiagnose/kubevirt-dpdk-checkup/pkg/internal/checkup/vmi"
	"github.com/kiagnose/kubevirt-dpdk-checkup/pkg/internal/config"
	"github.com/kiagnose/kubevirt-dpdk-checkup/pkg/internal/status"
)

type kubeVirtVMIClient interface {
	CreateVirtualMachineInstance(ctx context.Context,
		namespace string,
		vmi *kvcorev1.VirtualMachineInstance) (*kvcorev1.VirtualMachineInstance, error)
	GetVirtualMachineInstance(ctx context.Context, namespace, name string) (*kvcorev1.VirtualMachineInstance, error)
	DeleteVirtualMachineInstance(ctx context.Context, namespace, name string) error
	CreatePod(ctx context.Context, namespace string, pod *k8scorev1.Pod) (*k8scorev1.Pod, error)
	GetPod(ctx context.Context, namespace, name string) (*k8scorev1.Pod, error)
}

type Checkup struct {
	client              kubeVirtVMIClient
	namespace           string
	params              config.Config
	vmi                 *kvcorev1.VirtualMachineInstance
	trafficGeneratorPod *k8scorev1.Pod
}

const (
	VMINamePrefix     = "dpdk-vmi"
	TrexPodNamePrefix = "trex"
)

func New(client kubeVirtVMIClient, namespace string, checkupConfig config.Config) *Checkup {
	return &Checkup{
		client:    client,
		namespace: namespace,
		params:    checkupConfig,
		vmi:       newDPDKVMI(checkupConfig),
	}
}

func (c *Checkup) Setup(ctx context.Context) error {
	const errMessagePrefix = "setup"
	var err error

	c.trafficGeneratorPod, err = c.createTrafficGeneratorPod(ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", errMessagePrefix, err)
	}

	createdTrafficGeneratorPod, err := c.waitForPodRunningStatus(ctx, c.namespace, c.trafficGeneratorPod.Name)
	if err != nil {
		return fmt.Errorf("%s: %w", errMessagePrefix, err)
	}
	c.trafficGeneratorPod = createdTrafficGeneratorPod

	createdVMI, err := c.client.CreateVirtualMachineInstance(ctx, c.namespace, c.vmi)
	if err != nil {
		return err
	}
	c.vmi = createdVMI

	return nil
}

func (c *Checkup) Run(ctx context.Context) error {
	return nil
}

func (c *Checkup) Teardown(ctx context.Context) error {
	const errPrefix = "teardown"

	if err := c.deleteVMI(ctx); err != nil {
		return fmt.Errorf("%s: %w", errPrefix, err)
	}

	if err := c.waitForVMIDeletion(ctx); err != nil {
		return fmt.Errorf("%s: %w", errPrefix, err)
	}

	return nil
}

func (c *Checkup) Results() status.Results {
	return status.Results{}
}

func (c *Checkup) deleteVMI(ctx context.Context) error {
	if c.vmi == nil {
		return fmt.Errorf("failed to delete VMI, object doesn't exist")
	}

	vmiFullName := ObjectFullName(c.vmi.Namespace, c.vmi.Name)

	log.Printf("Trying to delete VMI: %q", vmiFullName)
	if err := c.client.DeleteVirtualMachineInstance(ctx, c.vmi.Namespace, c.vmi.Name); err != nil {
		log.Printf("Failed to delete VMI: %q", vmiFullName)
		return err
	}

	return nil
}

func (c *Checkup) waitForVMIDeletion(ctx context.Context) error {
	vmiFullName := ObjectFullName(c.vmi.Namespace, c.vmi.Name)
	log.Printf("Waiting for VMI %q to be deleted...", vmiFullName)

	conditionFn := func(ctx context.Context) (bool, error) {
		_, err := c.client.GetVirtualMachineInstance(ctx, c.vmi.Namespace, c.vmi.Name)
		if k8serrors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	}
	const pollInterval = 5 * time.Second
	if err := wait.PollImmediateUntilWithContext(ctx, pollInterval, conditionFn); err != nil {
		return fmt.Errorf("failed to wait for VMI %q to be deleted: %v", vmiFullName, err)
	}

	log.Printf("VMI %q was deleted successfully", vmiFullName)

	return nil
}

func ObjectFullName(namespace, name string) string {
	return fmt.Sprintf("%s/%s", namespace, name)
}

func newDPDKVMI(checkupConfig config.Config) *kvcorev1.VirtualMachineInstance {
	const (
		CPUSocketsCount   = 1
		CPUCoresCount     = 4
		CPUTreadsCount    = 2
		rootDiskName      = "rootdisk"
		cloudInitDiskName = "cloudinitdisk"
		userData          = `#cloud-config
user: cloud-user
password: 0tli-pxem-xknu
chpasswd:
  expire: false`

		eastNetworkName   = "nic-east"
		eastNICPCIAddress = "0000:06:00.0"
		westNetworkName   = "nic-west"
		westNICPCIAddress = "0000:07:00.0"

		terminationGracePeriodSeconds = 180
	)

	return vmi.New(randomizeName(VMINamePrefix),
		vmi.WithoutCRIOCPULoadBalancing(),
		vmi.WithoutCRIOCPUQuota(),
		vmi.WithoutCRIOIRQLoadBalancing(),
		vmi.WithDedicatedCPU(CPUSocketsCount, CPUCoresCount, CPUTreadsCount),
		vmi.WithSRIOVInterface(eastNetworkName, checkupConfig.DPDKEastMacAddress.String(), eastNICPCIAddress),
		vmi.WithMultusNetwork(eastNetworkName, checkupConfig.NetworkAttachmentDefinitionName),
		vmi.WithSRIOVInterface(westNetworkName, checkupConfig.DPDKWestMacAddress.String(), westNICPCIAddress),
		vmi.WithMultusNetwork(westNetworkName, checkupConfig.NetworkAttachmentDefinitionName),
		vmi.WithNetworkInterfaceMultiQueue(),
		vmi.WithRandomNumberGenerator(),
		vmi.WithHugePages(),
		vmi.WithMemoryRequest("8Gi"),
		vmi.WithTerminationGracePeriodSeconds(terminationGracePeriodSeconds),
		vmi.WithNodeSelector(checkupConfig.DPDKNodeLabelSelector),
		vmi.WithPVCVolume(rootDiskName, "rhel8-yummy-gorilla"),
		vmi.WithVirtIODisk(rootDiskName),
		vmi.WithCloudInitNoCloudVolume(cloudInitDiskName, userData),
		vmi.WithVirtIODisk(cloudInitDiskName),
	)
}

func randomizeName(prefix string) string {
	const randomStringLen = 5

	return fmt.Sprintf("%s-%s", prefix, k8srand.String(randomStringLen))
}

func (c *Checkup) createTrafficGeneratorPod(ctx context.Context) (*k8scorev1.Pod, error) {
	secondaryNetworksRequest, err := pod.CreateNetworksRequest([]networkv1.NetworkSelectionElement{
		{Name: c.params.NetworkAttachmentDefinitionName, Namespace: c.namespace, MacRequest: c.params.TrafficGeneratorEastMacAddress.String()},
		{Name: c.params.NetworkAttachmentDefinitionName, Namespace: c.namespace, MacRequest: c.params.TrafficGeneratorWestMacAddress.String()},
	})
	if err != nil {
		return nil, err
	}
	trexPod := newTrafficGeneratorPod(c.params, secondaryNetworksRequest)

	log.Printf("Creating Pod %s..", ObjectFullName(c.namespace, trexPod.Name))
	return c.client.CreatePod(ctx, c.namespace, trexPod)
}

func (c *Checkup) waitForPodRunningStatus(ctx context.Context, namespace, name string) (*k8scorev1.Pod, error) {
	podFullName := ObjectFullName(c.namespace, name)
	log.Printf("Waiting for Pod %s..", podFullName)
	var updatedPod *k8scorev1.Pod

	conditionFn := func(ctx context.Context) (bool, error) {
		var err error
		updatedPod, err = c.client.GetPod(ctx, namespace, name)
		if err != nil {
			return false, err
		}
		return pod.PodInRunningPhase(updatedPod), nil
	}
	const interval = time.Second * 5
	if err := wait.PollImmediateUntilWithContext(ctx, interval, conditionFn); err != nil {
		return nil, fmt.Errorf("failed to wait for POD '%s' to be in Running Phase: %v", podFullName, err)
	}

	log.Printf("Pod %s is Running", podFullName)
	return updatedPod, nil
}

func newTrafficGeneratorPod(checkupConfig config.Config, secondaryNetworkRequest string) *k8scorev1.Pod {
	const (
		TrexPodNumCPUs      = "8"
		TrexPodNumHugepages = "8Gi"
	)

	envVars := map[string]string{
		config.PortBandwidthGBParamName: fmt.Sprintf("%d", checkupConfig.PortBandwidthGB),
	}
	securityContext := pod.NewSecurityContext(int64(0), false,
		[]k8scorev1.Capability{"IPC_LOCK", "SYS_RESOURCE", "NET_RAW", "NET_ADMIN"})

	trexContainer := pod.NewPodContainer(TrexPodNamePrefix,
		pod.WithContainerImage(pod.ContainerDiskImage),
		pod.WithContainerCommand([]string{"/bin/bash", "-c", "sleep INF"}),
		pod.WithContainerSecurityContext(securityContext),
		pod.WithContainerEnvVars(envVars),
		pod.WithContainerCPUsStrict(TrexPodNumCPUs),
		pod.WithContainerHugepagesResources(TrexPodNumHugepages),
		pod.WithContainerHugepagesVolumeMount(),
	)

	return pod.NewPod(randomizeName(TrexPodNamePrefix),
		pod.WithPodContainer(trexContainer),
		pod.WithoutCRIOCPULoadBalancing(),
		pod.WithoutCRIOCPUQuota(),
		pod.WithoutCRIOIRQLoadBalancing(),
		pod.WithOwnerReference(checkupConfig.PodName, checkupConfig.PodUID),
		pod.WithNodeSelector(checkupConfig.TrafficGeneratorNodeLabelSelector),
		pod.WithNetworkRequestAnnotation(secondaryNetworkRequest),
		pod.WithHugepagesVolume(),
	)
}
