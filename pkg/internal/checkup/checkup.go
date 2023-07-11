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

	kvcorev1 "kubevirt.io/api/core/v1"

	kaffinity "github.com/kiagnose/kubevirt-dpdk-checkup/pkg/internal/checkup/affinity"
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
}

type testExecutor interface {
	Execute(ctx context.Context, vmiName string) (status.Results, error)
}

type Checkup struct {
	client    kubeVirtVMIClient
	namespace string
	params    config.Config
	vmi       *kvcorev1.VirtualMachineInstance
	results   status.Results
	executor  testExecutor
}

const (
	VMINamePrefix          = "dpdk-vmi"
	DPDKCheckupUIDLabelKey = "kubevirt-dpdk-checkup/uid"
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

func (c *Checkup) Setup(ctx context.Context) (setupErr error) {
	const errMessagePrefix = "setup"
	var err error

	if err = c.createVMI(ctx); err != nil {
		return fmt.Errorf("%s: %w", errMessagePrefix, err)
	}
	defer func() {
		if setupErr != nil {
			c.cleanupVMI()
		}
	}()

	err = c.waitForVMIToBoot(ctx)
	if err != nil {
		return err
	}

	return nil
}

func (c *Checkup) Run(ctx context.Context) error {
	var err error

	c.results, err = c.executor.Execute(ctx, c.vmi.Name)
	if err != nil {
		return err
	}
	c.results.DPDKVMNode = c.vmi.Status.NodeName

	if c.results.TrafficGeneratorOutErrorPackets != 0 || c.results.TrafficGeneratorInErrorPackets != 0 {
		return fmt.Errorf("detected Error Packets on the traffic generator's side: Oerrors %d Ierrors %d",
			c.results.TrafficGeneratorOutErrorPackets, c.results.TrafficGeneratorInErrorPackets)
	}

	if c.results.DPDKPacketsRxDropped != 0 || c.results.DPDKPacketsTxDropped != 0 {
		return fmt.Errorf("detected packets dropped on the DPDK VM's side: RX: %d; TX: %d",
			c.results.DPDKPacketsRxDropped, c.results.DPDKPacketsTxDropped)
	}

	if c.results.TrafficGeneratorTxPackets != c.results.DPDKRxTestPackets {
		return fmt.Errorf("not all generated packets had reached DPDK VM: Sent from traffic generator: %d; Received on DPDK VM: %d",
			c.results.TrafficGeneratorTxPackets, c.results.DPDKRxTestPackets)
	}

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
	var updatedVMI *kvcorev1.VirtualMachineInstance

	conditionFn := func(ctx context.Context) (bool, error) {
		var err error
		updatedVMI, err = c.client.GetVirtualMachineInstance(ctx, c.vmi.Namespace, c.vmi.Name)
		if err != nil {
			return false, err
		}

		for _, condition := range updatedVMI.Status.Conditions {
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
	c.vmi = updatedVMI
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

func (c *Checkup) cleanupVMI() {
	const setupCleanupTimeout = 30 * time.Second

	log.Printf("setup failed, cleanup VMI '%s/%s'", c.vmi.Namespace, c.vmi.Name)
	delCtx, cancel := context.WithTimeout(context.Background(), setupCleanupTimeout)
	defer cancel()
	_ = c.deleteVMI(delCtx)

	if derr := c.waitForVMIDeletion(delCtx); derr != nil {
		log.Printf("Failed to cleanup VMI '%s/%s': %v", c.vmi.Namespace, c.vmi.Name, derr)
	}
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
		hugePageSize      = "1Gi"
		guestMemory       = "4Gi"
		rootDiskName      = "rootdisk"
		cloudInitDiskName = "cloudinitdisk"
		eastNetworkName   = "nic-east"
		westNetworkName   = "nic-west"

		terminationGracePeriodSeconds = 0
	)

	labels := map[string]string{
		DPDKCheckupUIDLabelKey: checkupConfig.PodUID,
	}
	var affinity *k8scorev1.Affinity
	if checkupConfig.DPDKNodeLabelSelector != "" {
		affinity = &k8scorev1.Affinity{NodeAffinity: kaffinity.NewRequiredNodeAffinity(checkupConfig.DPDKNodeLabelSelector)}
	} else {
		affinity = &k8scorev1.Affinity{PodAntiAffinity: kaffinity.NewPreferredPodAntiAffinity(DPDKCheckupUIDLabelKey,
			checkupConfig.PodUID)}
	}

	return vmi.New(randomizeName(VMINamePrefix),
		vmi.WithOwnerReference(checkupConfig.PodName, checkupConfig.PodUID),
		vmi.WithLabels(labels),
		vmi.WithAffinity(affinity),
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
		vmi.WithMemory(hugePageSize, guestMemory),
		vmi.WithTerminationGracePeriodSeconds(terminationGracePeriodSeconds),
		vmi.WithContainerDisk(rootDiskName, checkupConfig.VMContainerDiskImage),
		vmi.WithVirtIODisk(rootDiskName),
		vmi.WithCloudInitNoCloudVolume(cloudInitDiskName, CloudInit(config.VMIUsername, config.VMIPassword)),
		vmi.WithVirtIODisk(cloudInitDiskName),
	)
}
