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
	"k8s.io/apimachinery/pkg/util/wait"

	kvcorev1 "kubevirt.io/api/core/v1"

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
	Execute(ctx context.Context, vmiUnderTestName string) (status.Results, error)
}

type Checkup struct {
	client       kubeVirtVMIClient
	namespace    string
	params       config.Config
	vmiUnderTest *kvcorev1.VirtualMachineInstance
	results      status.Results
	executor     testExecutor
}

const (
	VMIUnderTestNamePrefix = "vmi-under-test"
)

func New(client kubeVirtVMIClient, namespace string, checkupConfig config.Config, executor testExecutor) *Checkup {
	return &Checkup{
		client:       client,
		namespace:    namespace,
		params:       checkupConfig,
		vmiUnderTest: newVMIUnderTest(checkupConfig),
		executor:     executor,
	}
}

func (c *Checkup) Setup(ctx context.Context) (setupErr error) {
	const errMessagePrefix = "setup"
	var err error

	if err = c.createVMI(ctx, c.vmiUnderTest); err != nil {
		return fmt.Errorf("%s: %w", errMessagePrefix, err)
	}
	defer func() {
		if setupErr != nil {
			c.cleanupVMI(c.vmiUnderTest.Name)
		}
	}()

	var updatedVMIUnderTest *kvcorev1.VirtualMachineInstance
	updatedVMIUnderTest, err = c.waitForVMIToBoot(ctx, c.vmiUnderTest.Name)
	if err != nil {
		return err
	}

	c.vmiUnderTest = updatedVMIUnderTest

	return nil
}

func (c *Checkup) Run(ctx context.Context) error {
	var err error

	c.results, err = c.executor.Execute(ctx, c.vmiUnderTest.Name)
	if err != nil {
		return err
	}
	c.results.DPDKVMNode = c.vmiUnderTest.Status.NodeName

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

	if err := c.deleteVMI(ctx, c.vmiUnderTest.Name); err != nil {
		return fmt.Errorf("%s: %w", errPrefix, err)
	}

	if err := c.waitForVMIDeletion(ctx, c.vmiUnderTest.Name); err != nil {
		return fmt.Errorf("%s: %w", errPrefix, err)
	}

	return nil
}

func (c *Checkup) Results() status.Results {
	return c.results
}

func (c *Checkup) createVMI(ctx context.Context, vmiToCreate *kvcorev1.VirtualMachineInstance) error {
	log.Printf("Creating VMI %q...", ObjectFullName(c.namespace, vmiToCreate.Name))

	_, err := c.client.CreateVirtualMachineInstance(ctx, c.namespace, vmiToCreate)
	return err
}

func (c *Checkup) waitForVMIToBoot(ctx context.Context, name string) (*kvcorev1.VirtualMachineInstance, error) {
	vmiFullName := ObjectFullName(c.namespace, name)
	log.Printf("Waiting for VMI %q to boot...", vmiFullName)
	var updatedVMI *kvcorev1.VirtualMachineInstance

	conditionFn := func(ctx context.Context) (bool, error) {
		var err error
		updatedVMI, err = c.client.GetVirtualMachineInstance(ctx, c.namespace, name)
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
		return nil, fmt.Errorf("failed to wait for VMI %q to boot: %v", vmiFullName, err)
	}

	log.Printf("VMI %q had successfully booted", vmiFullName)

	return updatedVMI, nil
}

func (c *Checkup) deleteVMI(ctx context.Context, name string) error {
	vmiFullName := ObjectFullName(c.namespace, name)

	log.Printf("Trying to delete VMI: %q", vmiFullName)
	if err := c.client.DeleteVirtualMachineInstance(ctx, c.namespace, name); err != nil {
		log.Printf("Failed to delete VMI: %q", vmiFullName)
		return err
	}

	return nil
}

func (c *Checkup) waitForVMIDeletion(ctx context.Context, name string) error {
	vmiFullName := ObjectFullName(c.namespace, name)
	log.Printf("Waiting for VMI %q to be deleted...", vmiFullName)

	conditionFn := func(ctx context.Context) (bool, error) {
		_, err := c.client.GetVirtualMachineInstance(ctx, c.namespace, name)
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

func (c *Checkup) cleanupVMI(name string) {
	const setupCleanupTimeout = 30 * time.Second

	vmiFullName := ObjectFullName(c.namespace, name)
	log.Printf("setup failed, cleanup VMI %q", vmiFullName)

	delCtx, cancel := context.WithTimeout(context.Background(), setupCleanupTimeout)
	defer cancel()

	_ = c.deleteVMI(delCtx, name)

	if err := c.waitForVMIDeletion(delCtx, name); err != nil {
		log.Printf("Failed to wait for VMI %q disposal: %v", vmiFullName, err)
	}
}

func ObjectFullName(namespace, name string) string {
	return fmt.Sprintf("%s/%s", namespace, name)
}

func newVMIUnderTest(checkupConfig config.Config) *kvcorev1.VirtualMachineInstance {
	vmiConfig := vmi.DPDKVMIConfig{
		NamePrefix:                      VMIUnderTestNamePrefix,
		OwnerName:                       checkupConfig.PodName,
		OwnerUID:                        checkupConfig.PodUID,
		Affinity:                        vmi.Affinity(checkupConfig.DPDKNodeLabelSelector, checkupConfig.PodUID),
		ContainerDiskImage:              checkupConfig.VMContainerDiskImage,
		NetworkAttachmentDefinitionName: checkupConfig.NetworkAttachmentDefinitionName,
		NICEastMACAddress:               checkupConfig.DPDKEastMacAddress.String(),
		NICEastPCIAddress:               config.VMIEastNICPCIAddress,
		NICWestMACAddress:               checkupConfig.DPDKWestMacAddress.String(),
		NICWestPCIAddress:               config.VMIWestNICPCIAddress,
		Username:                        config.VMIUsername,
		Password:                        config.VMIPassword,
	}

	return vmi.NewDPDKVMI(vmiConfig)
}
