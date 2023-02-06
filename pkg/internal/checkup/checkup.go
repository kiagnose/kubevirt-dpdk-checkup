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
	"strings"
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
	DeletePod(ctx context.Context, namespace, name string) error
	GetPod(ctx context.Context, namespace, name string) (*k8scorev1.Pod, error)
}

type testExecutor interface {
	Execute(ctx context.Context, vmiName, podName, podContainerName string) (status.Results, error)
}

type Checkup struct {
	client              kubeVirtVMIClient
	namespace           string
	params              config.Config
	vmi                 *kvcorev1.VirtualMachineInstance
	trafficGeneratorPod *k8scorev1.Pod
	results             status.Results
	executor            testExecutor
}

const (
	VMINamePrefix                 = "dpdk-vmi"
	TrafficGeneratorPodNamePrefix = "kubevirt-dpdk-checkup-traffic-gen"
)

func New(client kubeVirtVMIClient, namespace string, checkupConfig config.Config, executor testExecutor) *Checkup {
	return &Checkup{
		client:    client,
		namespace: namespace,
		params:    checkupConfig,
		vmi:       newDPDKVMI(checkupConfig),
		executor:  executor,
	}
}

func (c *Checkup) Setup(ctx context.Context) error {
	const errMessagePrefix = "setup"
	var err error

	if err = c.createVMI(ctx); err != nil {
		return fmt.Errorf("%s: %w", errMessagePrefix, err)
	}

	if err = c.createTrafficGeneratorPod(ctx); err != nil {
		return fmt.Errorf("%s: %w", errMessagePrefix, err)
	}

	err = c.waitForVMIToBoot(ctx)
	if err != nil {
		return err
	}

	createdTrafficGeneratorPod, err := c.waitForPodRunningStatus(ctx, c.namespace, c.trafficGeneratorPod.Name)
	if err != nil {
		return fmt.Errorf("%s: %w", errMessagePrefix, err)
	}
	c.trafficGeneratorPod = createdTrafficGeneratorPod

	return nil
}

func (c *Checkup) Run(ctx context.Context) error {
	var err error

	c.results, err = c.executor.Execute(ctx, c.vmi.Name, c.trafficGeneratorPod.Name, c.trafficGeneratorPod.Spec.Containers[0].Name)
	if err != nil {
		return err
	}
	c.results.TrafficGeneratorNode = c.trafficGeneratorPod.Spec.NodeName
	c.results.DPDKVMNode = c.vmi.Status.NodeName

	if c.results.TrafficGeneratorMaxDropRate != 0 {
		return fmt.Errorf("detected %f Bps Drop Rate on the traffic generator's side", c.results.TrafficGeneratorMaxDropRate)
	}

	if c.results.TrafficGeneratorOutErrorPackets != 0 {
		return fmt.Errorf("detected %d Output Error Packets on the traffic generator's side", c.results.TrafficGeneratorOutErrorPackets)
	}

	if c.results.DPDKPacketsRxDropped != 0 {
		return fmt.Errorf("detected %d Rx packets dropped on the DPDK VM's side", c.results.DPDKPacketsRxDropped)
	}

	if c.results.DPDKPacketsTxDropped != 0 {
		return fmt.Errorf("detected %d Tx packets dropped on the DPDK VM's side", c.results.DPDKPacketsTxDropped)
	}

	return nil
}

func (c *Checkup) Teardown(ctx context.Context) error {
	const errPrefix = "teardown"

	if err := c.deleteVMI(ctx); err != nil {
		return fmt.Errorf("%s: %w", errPrefix, err)
	}

	if err := c.deletePod(ctx); err != nil {
		return fmt.Errorf("%s: %w", errPrefix, err)
	}

	if err := c.waitForVMIDeletion(ctx); err != nil {
		return fmt.Errorf("%s: %w", errPrefix, err)
	}

	if err := c.waitForPodDeletion(ctx); err != nil {
		return fmt.Errorf("%s: %w", errPrefix, err)
	}

	return nil
}

func (c *Checkup) Results() status.Results {
	return c.results
}

func (c *Checkup) createVMI(ctx context.Context) error {
	log.Printf("Creating VMI %q...", ObjectFullName(c.namespace, c.vmi.Name))

	var err error
	c.vmi, err = c.client.CreateVirtualMachineInstance(ctx, c.namespace, c.vmi)
	if err != nil {
		return err
	}

	return nil
}

func (c *Checkup) waitForVMIToBoot(ctx context.Context) error {
	vmiFullName := ObjectFullName(c.vmi.Namespace, c.vmi.Name)
	log.Printf("Waiting for VMI %q to boot...", vmiFullName)

	conditionFn := func(ctx context.Context) (bool, error) {
		fetchedVMI, err := c.client.GetVirtualMachineInstance(ctx, c.vmi.Namespace, c.vmi.Name)
		if err != nil {
			return false, err
		}

		for _, condition := range fetchedVMI.Status.Conditions {
			if condition.Type == kvcorev1.VirtualMachineInstanceAgentConnected && condition.Status == k8scorev1.ConditionTrue {
				return true, nil
			}
		}

		return false, nil
	}
	const pollInterval = 5 * time.Second
	if err := wait.PollImmediateUntilWithContext(ctx, pollInterval, conditionFn); err != nil {
		return fmt.Errorf("failed to wait for VMI %q to boot: %v", vmiFullName, err)
	}

	log.Printf("VMI %q had successfully booted", vmiFullName)

	return nil
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

func (c *Checkup) createTrafficGeneratorPod(ctx context.Context) error {
	secondaryNetworksRequest, err := pod.CreateNetworksRequest([]networkv1.NetworkSelectionElement{
		{Name: c.params.NetworkAttachmentDefinitionName, Namespace: c.namespace, MacRequest: c.params.TrafficGeneratorEastMacAddress.String()},
		{Name: c.params.NetworkAttachmentDefinitionName, Namespace: c.namespace, MacRequest: c.params.TrafficGeneratorWestMacAddress.String()},
	})
	if err != nil {
		return err
	}
	trafficGeneratorPod := newTrafficGeneratorPod(c.params, secondaryNetworksRequest)

	log.Printf("Creating traffic generator Pod %s..", ObjectFullName(c.namespace, trafficGeneratorPod.Name))
	c.trafficGeneratorPod, err = c.client.CreatePod(ctx, c.namespace, trafficGeneratorPod)
	if err != nil {
		return err
	}

	return nil
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
		return nil, fmt.Errorf("failed to wait for Pod '%s' to be in Running Phase: %v", podFullName, err)
	}

	log.Printf("Pod %s is Running", podFullName)
	return updatedPod, nil
}

func (c *Checkup) deletePod(ctx context.Context) error {
	if c.trafficGeneratorPod == nil {
		return fmt.Errorf("failed to delete traffic generator Pod, object doesn't exist")
	}

	vmiFullName := ObjectFullName(c.trafficGeneratorPod.Namespace, c.trafficGeneratorPod.Name)

	log.Printf("Trying to delete traffic generator Pod: %q", vmiFullName)
	if err := c.client.DeletePod(ctx, c.trafficGeneratorPod.Namespace, c.trafficGeneratorPod.Name); err != nil {
		log.Printf("Failed to delete traffic generator Pod: %q", vmiFullName)
		return err
	}

	return nil
}

func (c *Checkup) waitForPodDeletion(ctx context.Context) error {
	podFullName := ObjectFullName(c.trafficGeneratorPod.Namespace, c.trafficGeneratorPod.Name)
	log.Printf("Waiting for Pod %q to be deleted..", podFullName)

	conditionFn := func(ctx context.Context) (bool, error) {
		var err error
		_, err = c.client.GetPod(ctx, c.trafficGeneratorPod.Namespace, c.trafficGeneratorPod.Name)
		if k8serrors.IsNotFound(err) {
			return true, nil
		}
		return false, err
	}
	const interval = 5 * time.Second
	if err := wait.PollImmediateUntilWithContext(ctx, interval, conditionFn); err != nil {
		return fmt.Errorf("failed to wait for POD %q to be in deleted: %v", podFullName, err)
	}

	log.Printf("Pod %q is deleted", podFullName)
	return nil
}

func ObjectFullName(namespace, name string) string {
	return fmt.Sprintf("%s/%s", namespace, name)
}

func CloudInit(username, password string) string {
	sb := strings.Builder{}
	sb.WriteString("#cloud-config\n")
	sb.WriteString(fmt.Sprintf("user: %s\n", username))
	sb.WriteString(fmt.Sprintf("password: %s\n", password))
	sb.WriteString("chpasswd:\n")
	sb.WriteString("  expire: false")

	return sb.String()
}

func randomizeName(prefix string) string {
	const randomStringLen = 5

	return fmt.Sprintf("%s-%s", prefix, k8srand.String(randomStringLen))
}

func newDPDKVMI(checkupConfig config.Config) *kvcorev1.VirtualMachineInstance {
	const (
		CPUSocketsCount   = 1
		CPUCoresCount     = 4
		CPUTreadsCount    = 2
		rootDiskName      = "rootdisk"
		cloudInitDiskName = "cloudinitdisk"
		eastNetworkName   = "nic-east"
		westNetworkName   = "nic-west"

		terminationGracePeriodSeconds = 0
	)

	return vmi.New(randomizeName(VMINamePrefix),
		vmi.WithOwnerReference(checkupConfig.PodName, checkupConfig.PodUID),
		vmi.WithoutCRIOCPULoadBalancing(),
		vmi.WithoutCRIOCPUQuota(),
		vmi.WithoutCRIOIRQLoadBalancing(),
		vmi.WithDedicatedCPU(CPUSocketsCount, CPUCoresCount, CPUTreadsCount),
		vmi.WithSRIOVInterface(eastNetworkName, checkupConfig.DPDKEastMacAddress.String(), config.VMIEastNICPCIAddress),
		vmi.WithMultusNetwork(eastNetworkName, checkupConfig.NetworkAttachmentDefinitionName),
		vmi.WithSRIOVInterface(westNetworkName, checkupConfig.DPDKWestMacAddress.String(), config.VMIWestNICPCIAddress),
		vmi.WithMultusNetwork(westNetworkName, checkupConfig.NetworkAttachmentDefinitionName),
		vmi.WithNetworkInterfaceMultiQueue(),
		vmi.WithRandomNumberGenerator(),
		vmi.WithHugePages(),
		vmi.WithMemoryRequest("8Gi"),
		vmi.WithTerminationGracePeriodSeconds(terminationGracePeriodSeconds),
		vmi.WithNodeSelector(checkupConfig.DPDKNodeLabelSelector),
		vmi.WithPVCVolume(rootDiskName, "rhel8-yummy-gorilla"),
		vmi.WithVirtIODisk(rootDiskName),
		vmi.WithCloudInitNoCloudVolume(cloudInitDiskName, CloudInit(config.VMIUsername, config.VMIPassword)),
		vmi.WithVirtIODisk(cloudInitDiskName),
	)
}

func newTrafficGeneratorPod(checkupConfig config.Config, secondaryNetworkRequest string) *k8scorev1.Pod {
	const (
		trafficGeneratorPodCPUCount            = 8
		trafficGeneratorPodNumOfNonTrafficCPUs = 2
		trafficGeneratorPodHugepagesCount      = "8Gi"
		terminationGracePeriodSeconds          = int64(0)

		portBandwidthParamName     = "PORT_BANDWIDTH_GB"
		numaSocketParamName        = "NUMA_SOCKET"
		verboseParamName           = "SET_VERBOSE"
		numTrafficCpusParamName    = "NUM_OF_TRAFFIC_CPUS"
		numCpusParamName           = "NUM_OF_CPUS"
		srcWestMACAddressParamName = "SRC_WEST_MAC_ADDRESS"
		srcEastMACAddressParamName = "SRC_EAST_MAC_ADDRESS"
		dstWestMACAddressParamName = "DST_WEST_MAC_ADDRESS"
		dstEastMACAddressParamName = "DST_EAST_MAC_ADDRESS"
	)

	envVars := map[string]string{
		portBandwidthParamName:     fmt.Sprintf("%d", checkupConfig.PortBandwidthGB),
		numaSocketParamName:        fmt.Sprintf("%d", checkupConfig.NUMASocket),
		numTrafficCpusParamName:    fmt.Sprintf("%d", trafficGeneratorPodCPUCount-trafficGeneratorPodNumOfNonTrafficCPUs),
		numCpusParamName:           fmt.Sprintf("%d", trafficGeneratorPodCPUCount),
		srcWestMACAddressParamName: checkupConfig.TrafficGeneratorWestMacAddress.String(),
		srcEastMACAddressParamName: checkupConfig.TrafficGeneratorEastMacAddress.String(),
		dstWestMACAddressParamName: checkupConfig.DPDKWestMacAddress.String(),
		dstEastMACAddressParamName: checkupConfig.DPDKEastMacAddress.String(),
		verboseParamName:           "FALSE",
	}
	securityContext := pod.NewSecurityContext(int64(0), false,
		[]k8scorev1.Capability{"IPC_LOCK", "SYS_RESOURCE", "NET_RAW", "NET_ADMIN"})

	trafficGeneratorContainer := pod.NewPodContainer(TrafficGeneratorPodNamePrefix,
		pod.WithContainerImage(checkupConfig.TrafficGeneratorImage),
		pod.WithContainerCommand([]string{"/bin/bash", "/opt/scripts/main.sh"}),
		pod.WithContainerSecurityContext(securityContext),
		pod.WithContainerEnvVars(envVars),
		pod.WithContainerCPUsStrict(fmt.Sprintf("%d", trafficGeneratorPodCPUCount)),
		pod.WithContainerHugepagesResources(trafficGeneratorPodHugepagesCount),
		pod.WithContainerHugepagesVolumeMount(),
		pod.WithContainerLibModulesVolumeMount(),
	)

	return pod.NewPod(randomizeName(TrafficGeneratorPodNamePrefix),
		pod.WithPodContainer(trafficGeneratorContainer),
		pod.WithRuntimeClassName(checkupConfig.TrafficGeneratorRuntimeClassName),
		pod.WithoutCRIOCPULoadBalancing(),
		pod.WithoutCRIOCPUQuota(),
		pod.WithoutCRIOIRQLoadBalancing(),
		pod.WithOwnerReference(checkupConfig.PodName, checkupConfig.PodUID),
		pod.WithNodeSelector(checkupConfig.TrafficGeneratorNodeLabelSelector),
		pod.WithNetworkRequestAnnotation(secondaryNetworkRequest),
		pod.WithHugepagesVolume(),
		pod.WithLibModulesVolume(),
		pod.WithTerminationGracePeriodSeconds(terminationGracePeriodSeconds),
	)
}
