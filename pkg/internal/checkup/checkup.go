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
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/util/wait"

	kvcorev1 "kubevirt.io/api/core/v1"

	"github.com/kiagnose/kubevirt-dpdk-checkup/pkg/internal/checkup/configmap"
	"github.com/kiagnose/kubevirt-dpdk-checkup/pkg/internal/checkup/trex"
	"github.com/kiagnose/kubevirt-dpdk-checkup/pkg/internal/config"
	"github.com/kiagnose/kubevirt-dpdk-checkup/pkg/internal/status"
)

type kubeVirtVMIClient interface {
	CreateVirtualMachineInstance(ctx context.Context,
		namespace string,
		vmi *kvcorev1.VirtualMachineInstance) (*kvcorev1.VirtualMachineInstance, error)
	GetVirtualMachineInstance(ctx context.Context, namespace, name string) (*kvcorev1.VirtualMachineInstance, error)
	DeleteVirtualMachineInstance(ctx context.Context, namespace, name string) error
	CreateConfigMap(ctx context.Context, namespace string, configMap *k8scorev1.ConfigMap) (*k8scorev1.ConfigMap, error)
	DeleteConfigMap(ctx context.Context, namespace, name string) error
}

type testExecutor interface {
	Execute(ctx context.Context, vmiUnderTestName, trafficGenVMIName string) (status.Results, error)
}

type Checkup struct {
	client              kubeVirtVMIClient
	namespace           string
	params              config.Config
	vmiUnderTest        *kvcorev1.VirtualMachineInstance
	trafficGen          *kvcorev1.VirtualMachineInstance
	trafficGenConfigMap *k8scorev1.ConfigMap
	results             status.Results
	executor            testExecutor
}

const (
	TrafficGenConfigMapNamePrefix = "dpdk-traffic-gen-config"
)

func New(client kubeVirtVMIClient, namespace string, checkupConfig config.Config, executor testExecutor) *Checkup {
	const randomStringLen = 5
	randomSuffix := rand.String(randomStringLen)

	trafficGenCMName := trafficGenConfigMapName(randomSuffix)

	return &Checkup{
		client:              client,
		namespace:           namespace,
		params:              checkupConfig,
		vmiUnderTest:        newVMIUnderTest(vmiUnderTestName(randomSuffix), checkupConfig),
		trafficGen:          newTrafficGen(trafficGenName(randomSuffix), checkupConfig, trafficGenCMName),
		trafficGenConfigMap: newTrafficGenConfigMap(trafficGenCMName, checkupConfig),
		executor:            executor,
	}
}

func (c *Checkup) Setup(ctx context.Context) (setupErr error) {
	const errMessagePrefix = "setup"
	var err error

	if err = c.createTrafficGenCM(ctx); err != nil {
		return fmt.Errorf("%s: %w", errMessagePrefix, err)
	}

	if err = c.createVMI(ctx, c.vmiUnderTest); err != nil {
		return fmt.Errorf("%s: %w", errMessagePrefix, err)
	}
	defer func() {
		if setupErr != nil {
			c.cleanupVMI(c.vmiUnderTest.Name)
		}
	}()

	if err = c.createVMI(ctx, c.trafficGen); err != nil {
		return fmt.Errorf("%s: %w", errMessagePrefix, err)
	}
	defer func() {
		if setupErr != nil {
			c.cleanupVMI(c.trafficGen.Name)
		}
	}()

	var updatedVMIUnderTest *kvcorev1.VirtualMachineInstance
	updatedVMIUnderTest, err = c.waitForVMIToBoot(ctx, c.vmiUnderTest.Name)
	if err != nil {
		return err
	}

	c.vmiUnderTest = updatedVMIUnderTest

	var updatedTrafficGen *kvcorev1.VirtualMachineInstance
	updatedTrafficGen, err = c.waitForVMIToBoot(ctx, c.trafficGen.Name)
	if err != nil {
		return err
	}

	c.trafficGen = updatedTrafficGen

	return nil
}

func (c *Checkup) Run(ctx context.Context) error {
	var err error

	c.results, err = c.executor.Execute(ctx, c.vmiUnderTest.Name, c.trafficGen.Name)
	if err != nil {
		return err
	}
	c.results.VMUnderTestActualNodeName = c.vmiUnderTest.Status.NodeName
	c.results.TrafficGenActualNodeName = c.trafficGen.Status.NodeName

	var runErrors []string
	if c.results.TrafficGenSentPackets == 0 {
		runErrors = append(runErrors, "no packets were sent from the traffic generator")
	}

	if c.results.TrafficGenOutputErrorPackets != 0 || c.results.TrafficGenInputErrorPackets != 0 {
		runErrors = append(runErrors, fmt.Sprintf("detected Error Packets on the traffic generator's side: Oerrors %d Ierrors %d",
			c.results.TrafficGenOutputErrorPackets, c.results.TrafficGenInputErrorPackets))
	}

	if c.results.VMUnderTestRxDroppedPackets != 0 || c.results.VMUnderTestTxDroppedPackets != 0 {
		runErrors = append(runErrors, fmt.Sprintf("detected packets dropped on the VM-Under-Test's side: RX: %d; TX: %d",
			c.results.VMUnderTestRxDroppedPackets, c.results.VMUnderTestTxDroppedPackets))
	}

	if c.results.TrafficGenSentPackets != c.results.VMUnderTestReceivedPackets {
		runErrors = append(runErrors, fmt.Sprintf("not all generated packets had reached VM-Under-Test: "+
			"Sent from traffic generator: %d; Received on VM-Under-Test: %d",
			c.results.TrafficGenSentPackets,
			c.results.VMUnderTestReceivedPackets),
		)
	}

	if len(runErrors) > 0 {
		return fmt.Errorf("%s", strings.Join(runErrors, ", "))
	}
	return nil
}

func (c *Checkup) Teardown(ctx context.Context) error {
	const errPrefix = "teardown"

	if err := c.deleteVMI(ctx, c.vmiUnderTest.Name); err != nil {
		return fmt.Errorf("%s: %w", errPrefix, err)
	}

	if err := c.deleteVMI(ctx, c.trafficGen.Name); err != nil {
		return fmt.Errorf("%s: %w", errPrefix, err)
	}

	if err := c.deleteTrafficGenCM(ctx); err != nil {
		return fmt.Errorf("%s: %w", errPrefix, err)
	}

	if err := c.waitForVMIDeletion(ctx, c.vmiUnderTest.Name); err != nil {
		return fmt.Errorf("%s: %w", errPrefix, err)
	}

	if err := c.waitForVMIDeletion(ctx, c.trafficGen.Name); err != nil {
		return fmt.Errorf("%s: %w", errPrefix, err)
	}

	return nil
}

func (c *Checkup) Results() status.Results {
	return c.results
}

func (c *Checkup) createTrafficGenCM(ctx context.Context) error {
	log.Printf("Creating ConfigMap %q...", ObjectFullName(c.namespace, c.trafficGenConfigMap.Name))

	_, err := c.client.CreateConfigMap(ctx, c.namespace, c.trafficGenConfigMap)
	return err
}

func (c *Checkup) deleteTrafficGenCM(ctx context.Context) error {
	log.Printf("Deleting ConfigMap %q...", ObjectFullName(c.namespace, c.trafficGenConfigMap.Name))

	return c.client.DeleteConfigMap(ctx, c.namespace, c.trafficGenConfigMap.Name)
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

func newTrafficGenConfigMap(name string, checkupConfig config.Config) *k8scorev1.ConfigMap {
	trexConfig := trex.NewConfig(checkupConfig)
	trafficGenConfigData := map[string]string{
		trex.SystemdUnitFileName:        trex.GenerateSystemdUnitFile(),
		trex.ExecutionScriptName:        trexConfig.GenerateExecutionScript(),
		trex.CfgFileName:                trexConfig.GenerateCfgFile(),
		trex.StreamPyFileName:           trexConfig.GenerateStreamPyFile(),
		trex.StreamPeerParamsPyFileName: trexConfig.GenerateStreamAddrPyFile(),
	}
	return configmap.New(
		name,
		checkupConfig.PodName,
		checkupConfig.PodUID,
		trafficGenConfigData,
	)
}

func vmiUnderTestName(suffix string) string {
	return VMIUnderTestNamePrefix + "-" + suffix
}

func trafficGenName(suffix string) string {
	return TrafficGenNamePrefix + "-" + suffix
}

func trafficGenConfigMapName(suffix string) string {
	return TrafficGenConfigMapNamePrefix + "-" + suffix
}
