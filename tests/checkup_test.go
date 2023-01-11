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

package tests

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	testServiceAccountName = "default"
	testCheckupJobName     = "dpdk-checkup"
)

var _ = Describe("Execute the checkup Job", func() {
	var checkupJob *batchv1.Job

	BeforeEach(func() {
		var err error

		checkupJob = newCheckupJob()
		_, err = virtClient.BatchV1().Jobs(testNamespace).Create(context.Background(), checkupJob, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())

		DeferCleanup(func() {
			backgroundPropagationPolicy := metav1.DeletePropagationBackground
			err = virtClient.BatchV1().Jobs(testNamespace).Delete(context.Background(), testCheckupJobName, metav1.DeleteOptions{PropagationPolicy: &backgroundPropagationPolicy})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	It("should complete successfully", func() {
		Eventually(getJobConditions, 5*time.Minute, 5*time.Second).Should(
			ContainElement(MatchFields(IgnoreExtras, Fields{
				"Type":   Equal(batchv1.JobComplete),
				"Status": Equal(corev1.ConditionTrue),
			})))
	})
})

func getJobConditions() []batchv1.JobCondition {
	checkupJob, err := virtClient.BatchV1().Jobs(testNamespace).Get(context.Background(), testCheckupJobName, metav1.GetOptions{})
	if err != nil {
		return []batchv1.JobCondition{}
	}

	return checkupJob.Status.Conditions
}

func newCheckupJob() *batchv1.Job {
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name: testCheckupJobName,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: pointer(int32(0)),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					ServiceAccountName: testServiceAccountName,
					RestartPolicy:      corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Name:            testCheckupJobName,
							Image:           testImageName,
							SecurityContext: newSecurityContext(),
						},
					},
				},
			},
		},
	}
}

func newSecurityContext() *corev1.SecurityContext {
	return &corev1.SecurityContext{
		AllowPrivilegeEscalation: pointer(false),
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{"ALL"},
		},
		RunAsNonRoot: pointer(true),
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		},
	}
}

func pointer[T any](v T) *T {
	return &v
}
