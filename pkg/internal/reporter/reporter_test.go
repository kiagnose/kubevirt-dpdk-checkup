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

package reporter_test

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	assert "github.com/stretchr/testify/require"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"

	kconfigmap "github.com/kiagnose/kiagnose/kiagnose/configmap"

	"github.com/kiagnose/kubevirt-dpdk-checkup/pkg/internal/reporter"
	"github.com/kiagnose/kubevirt-dpdk-checkup/pkg/internal/status"
)

const (
	testNamespace     = "target-ns"
	testConfigMapName = "dpdk-checkup-config"
)

func TestReportShouldSucceed(t *testing.T) {
	fakeClient := fake.NewSimpleClientset(newConfigMap())
	testReporter := reporter.New(fakeClient, testNamespace, testConfigMapName)

	assert.NoError(t, testReporter.Report(status.Status{}))
}

type checkupFailureCase struct {
	description    string
	failureReasons []string
}

func TestReportShouldSuccessfullyReportResults(t *testing.T) {
	t.Run("on checkup success", func(t *testing.T) {
		const (
			expectedTrafficGenSentPackets        = 0
			expectedTrafficGenOutputErrorPackets = 0
			expectedTrafficGenInputErrorPackets  = 0
			expectedVMUnderTestReceivedPackets   = 0
			expectedVMUnderTestRxDroppedPackets  = 0
			expectedVMUnderTestTxDroppedPackets  = 0
			expectedVMUnderTestActualNodeName    = "dpdk-node01"
			expectedTrafficGenActualNodeName     = "dpdk-node02"
		)
		fakeClient := fake.NewSimpleClientset(newConfigMap())
		testReporter := reporter.New(fakeClient, testNamespace, testConfigMapName)

		var checkupStatus status.Status
		checkupStatus.StartTimestamp = time.Now()
		assert.NoError(t, testReporter.Report(checkupStatus))

		checkupStatus.FailureReason = []string{}
		checkupStatus.CompletionTimestamp = time.Now()
		checkupStatus.Results = status.Results{
			TrafficGenSentPackets:        expectedTrafficGenSentPackets,
			TrafficGenOutputErrorPackets: expectedTrafficGenOutputErrorPackets,
			TrafficGenInputErrorPackets:  expectedTrafficGenInputErrorPackets,
			VMUnderTestReceivedPackets:   expectedVMUnderTestReceivedPackets,
			VMUnderTestRxDroppedPackets:  expectedVMUnderTestRxDroppedPackets,
			VMUnderTestTxDroppedPackets:  expectedVMUnderTestTxDroppedPackets,
			VMUnderTestActualNodeName:    expectedVMUnderTestActualNodeName,
			TrafficGenActualNodeName:     expectedTrafficGenActualNodeName,
		}

		assert.NoError(t, testReporter.Report(checkupStatus))
		expectedReportData := createExpectedReporterConfigmapDataWithResults(true, checkupStatus)
		assert.Equal(t, expectedReportData, getCheckupData(t, fakeClient, testNamespace, testConfigMapName))
	})

	t.Run("on checkup failure", func(t *testing.T) {
		const (
			failureReason1 = "some reason"
			failureReason2 = "some other reason"
		)

		testCases := []checkupFailureCase{
			{
				description:    "with no results",
				failureReasons: []string{failureReason1},
			},
			{
				description:    "with no results and multiple failures",
				failureReasons: []string{failureReason1, failureReason2},
			},
		}

		for _, testCase := range testCases {
			t.Run(testCase.description, func(t *testing.T) {
				fakeClient := fake.NewSimpleClientset(newConfigMap())
				testReporter := reporter.New(fakeClient, testNamespace, testConfigMapName)

				var checkupStatus status.Status
				checkupStatus.StartTimestamp = time.Now()
				assert.NoError(t, testReporter.Report(checkupStatus))

				checkupStatus.CompletionTimestamp = time.Now()
				checkupStatus.FailureReason = testCase.failureReasons
				expectedReportData := createBasicExpectedReporterConfigmapData(false, checkupStatus)
				assert.NoError(t, testReporter.Report(checkupStatus))
				assert.Equal(t, expectedReportData, getCheckupData(t, fakeClient, testNamespace, testConfigMapName))
			})
		}
	})
}

func TestReportShouldFailWhenCannotUpdateConfigMap(t *testing.T) {
	// ConfigMap does not exist
	fakeClient := fake.NewSimpleClientset()

	testReporter := reporter.New(fakeClient, testNamespace, testConfigMapName)

	assert.ErrorContains(t, testReporter.Report(status.Status{}), "not found")
}

func createBasicExpectedReporterConfigmapData(succeeded bool, checkupStatus status.Status) map[string]string {
	return map[string]string{
		"status.succeeded":           strconv.FormatBool(succeeded),
		"status.failureReason":       strings.Join(checkupStatus.FailureReason, ","),
		"status.startTimestamp":      timestamp(checkupStatus.StartTimestamp),
		"status.completionTimestamp": timestamp(checkupStatus.CompletionTimestamp),
	}
}

func createExpectedReporterConfigmapDataWithResults(succeeded bool, checkupStatus status.Status) map[string]string {
	results := createBasicExpectedReporterConfigmapData(succeeded, checkupStatus)
	results["status.result.trafficGenSentPackets"] = fmt.Sprintf("%d", checkupStatus.Results.TrafficGenSentPackets)
	results["status.result.trafficGenOutputErrorPackets"] = fmt.Sprintf("%d", checkupStatus.Results.TrafficGenOutputErrorPackets)
	results["status.result.trafficGenInputErrorPackets"] = fmt.Sprintf("%d", checkupStatus.Results.TrafficGenInputErrorPackets)
	results["status.result.vmUnderTestReceivedPackets"] = fmt.Sprintf("%d", checkupStatus.Results.VMUnderTestReceivedPackets)
	results["status.result.vmUnderTestRxDroppedPackets"] = fmt.Sprintf("%d", checkupStatus.Results.VMUnderTestRxDroppedPackets)
	results["status.result.vmUnderTestTxDroppedPackets"] = fmt.Sprintf("%d", checkupStatus.Results.VMUnderTestTxDroppedPackets)
	results["status.result.trafficGenActualNodeName"] = checkupStatus.Results.TrafficGenActualNodeName
	results["status.result.vmUnderTestActualNodeName"] = checkupStatus.Results.VMUnderTestActualNodeName
	return results
}

func getCheckupData(t *testing.T, client kubernetes.Interface, configMapNamespace, configMapName string) map[string]string {
	configMap, err := kconfigmap.Get(client, configMapNamespace, configMapName)
	assert.NoError(t, err)

	return configMap.Data
}

func newConfigMap() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testConfigMapName,
			Namespace: testNamespace,
		},
		Data: map[string]string{},
	}
}

func timestamp(t time.Time) string {
	return t.Format(time.RFC3339)
}
