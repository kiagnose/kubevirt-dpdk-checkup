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
	"encoding/json"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	testServiceAccountName              = "dpdk-checkup-sa"
	testKiagnoseConfigMapAccessRoleName = "kiagnose-configmap-access"
	testKubeVirtDPDKCheckerRoleName     = "kubevirt-dpdk-checker"
	testConfigMapName                   = "dpdk-checkup-config"
	testCheckupJobName                  = "dpdk-checkup"

	testTimeout = time.Hour
	jobGrace    = 5 * time.Minute
	jobTimeout  = testTimeout + jobGrace
)

var _ = Describe("Execute the checkup Job", func() {
	var (
		configMap  *corev1.ConfigMap
		checkupJob *batchv1.Job
	)

	BeforeEach(func() {
		setupCheckupPermissions()

		var err error
		configMap = newConfigMap()
		configMap, err = virtClient.CoreV1().ConfigMaps(testNamespace).Create(context.Background(), configMap, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())

		DeferCleanup(func() {
			err = virtClient.CoreV1().ConfigMaps(configMap.Namespace).Delete(context.Background(), configMap.Name, metav1.DeleteOptions{})
			Expect(err).NotTo(HaveOccurred())
		})
		checkupJob = newCheckupJob()
		checkupJob, err = virtClient.BatchV1().Jobs(testNamespace).Create(context.Background(), checkupJob, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())

		DeferCleanup(func() {
			backgroundPropagationPolicy := metav1.DeletePropagationBackground
			err = virtClient.BatchV1().Jobs(checkupJob.Namespace).Delete(
				context.Background(),
				checkupJob.Name,
				metav1.DeleteOptions{PropagationPolicy: &backgroundPropagationPolicy},
			)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	It("should complete successfully", func() {
		Eventually(func() []batchv1.JobCondition {
			jobConditions, err := getJobConditions()
			Expect(err).ToNot(HaveOccurred())

			for _, jobCondition := range jobConditions {
				if jobCondition.Type == batchv1.JobFailed && jobCondition.Status == corev1.ConditionTrue {
					configMap, err := virtClient.CoreV1().ConfigMaps(testNamespace).Get(context.Background(), testConfigMapName, metav1.GetOptions{})
					Expect(err).NotTo(HaveOccurred())

					Fail(fmt.Sprintf("checkup failed: %+v", prettifyData(configMap.Data)))
				}
			}

			return jobConditions
		}, jobTimeout, 5*time.Second).Should(
			ContainElement(MatchFields(IgnoreExtras, Fields{
				"Type":   Equal(batchv1.JobComplete),
				"Status": Equal(corev1.ConditionTrue),
			})), "checkup timed out")

		configMap, err := virtClient.CoreV1().ConfigMaps(testNamespace).Get(context.Background(), testConfigMapName, metav1.GetOptions{})
		Expect(err).NotTo(HaveOccurred())

		Expect(configMap.Data).NotTo(BeNil())
		Expect(configMap.Data["status.succeeded"]).To(Equal("true"), fmt.Sprintf("should succeed %+v", prettifyData(configMap.Data)))
		Expect(configMap.Data["status.failureReason"]).To(BeEmpty(), fmt.Sprintf("should be empty %+v", prettifyData(configMap.Data)))
		Expect(configMap.Data["status.result.vmUnderTestActualNodeName"]).ToNot(BeEmpty(),
			fmt.Sprintf("vmUnderTestActualNodeName should not be empty %+v", prettifyData(configMap.Data)))
		Expect(configMap.Data["status.result.trafficGenActualNodeName"]).ToNot(BeEmpty(),
			fmt.Sprintf("trafficGenActualNodeName should not be empty %+v", prettifyData(configMap.Data)))
	})
})

func prettifyData(data map[string]string) string {
	dataPrettyJSON, err := json.MarshalIndent(data, "", "\t")
	Expect(err).NotTo(HaveOccurred())
	return string(dataPrettyJSON)
}

func setupCheckupPermissions() {
	var (
		err                                error
		checkupServiceAccount              *corev1.ServiceAccount
		kiagnoseConfigMapAccessRole        *rbacv1.Role
		kiagnoseConfigMapAccessRoleBinding *rbacv1.RoleBinding
		kubeVirtDPDKCheckerRole            *rbacv1.Role
		kubeVirtDPDKCheckerRoleBinding     *rbacv1.RoleBinding
	)

	checkupServiceAccount = newServiceAccount(testServiceAccountName)
	checkupServiceAccount, err = virtClient.CoreV1().ServiceAccounts(testNamespace).Create(
		context.Background(),
		checkupServiceAccount,
		metav1.CreateOptions{},
	)
	Expect(err).NotTo(HaveOccurred())

	DeferCleanup(func() {
		err = virtClient.CoreV1().ServiceAccounts(checkupServiceAccount.Namespace).Delete(
			context.Background(),
			checkupServiceAccount.Name,
			metav1.DeleteOptions{},
		)
		Expect(err).NotTo(HaveOccurred())
	})

	kiagnoseConfigMapAccessRole = newKiagnoseConfigMapAccessRole(testKiagnoseConfigMapAccessRoleName)
	kiagnoseConfigMapAccessRole, err = virtClient.RbacV1().Roles(testNamespace).Create(
		context.Background(),
		kiagnoseConfigMapAccessRole,
		metav1.CreateOptions{},
	)
	Expect(err).NotTo(HaveOccurred())

	DeferCleanup(func() {
		err = virtClient.RbacV1().Roles(kiagnoseConfigMapAccessRole.Namespace).Delete(
			context.Background(),
			kiagnoseConfigMapAccessRole.Name,
			metav1.DeleteOptions{},
		)
		Expect(err).NotTo(HaveOccurred())
	})

	kiagnoseConfigMapAccessRoleBinding = newRoleBinding(
		kiagnoseConfigMapAccessRole.Name,
		checkupServiceAccount.Name,
		kiagnoseConfigMapAccessRole.Name)
	kiagnoseConfigMapAccessRoleBinding, err = virtClient.RbacV1().RoleBindings(testNamespace).Create(
		context.Background(),
		kiagnoseConfigMapAccessRoleBinding,
		metav1.CreateOptions{},
	)
	Expect(err).NotTo(HaveOccurred())

	DeferCleanup(func() {
		err = virtClient.RbacV1().RoleBindings(kiagnoseConfigMapAccessRoleBinding.Namespace).Delete(
			context.Background(),
			kiagnoseConfigMapAccessRoleBinding.Name,
			metav1.DeleteOptions{},
		)
		Expect(err).NotTo(HaveOccurred())
	})

	kubeVirtDPDKCheckerRole = newKubeVirtDPDKCheckerRole()
	kubeVirtDPDKCheckerRole, err = virtClient.RbacV1().Roles(testNamespace).Create(
		context.Background(),
		kubeVirtDPDKCheckerRole,
		metav1.CreateOptions{},
	)
	Expect(err).NotTo(HaveOccurred())

	DeferCleanup(func() {
		err = virtClient.RbacV1().Roles(kubeVirtDPDKCheckerRole.Namespace).Delete(
			context.Background(),
			kubeVirtDPDKCheckerRole.Name,
			metav1.DeleteOptions{},
		)
		Expect(err).NotTo(HaveOccurred())
	})

	kubeVirtDPDKCheckerRoleBinding = newRoleBinding(kubeVirtDPDKCheckerRole.Name, checkupServiceAccount.Name, kubeVirtDPDKCheckerRole.Name)
	kubeVirtDPDKCheckerRoleBinding, err = virtClient.RbacV1().RoleBindings(testNamespace).Create(
		context.Background(),
		kubeVirtDPDKCheckerRoleBinding,
		metav1.CreateOptions{},
	)
	Expect(err).NotTo(HaveOccurred())

	DeferCleanup(func() {
		err = virtClient.RbacV1().RoleBindings(kubeVirtDPDKCheckerRoleBinding.Namespace).Delete(
			context.Background(),
			kubeVirtDPDKCheckerRoleBinding.Name,
			metav1.DeleteOptions{},
		)
		Expect(err).NotTo(HaveOccurred())
	})
}

func newServiceAccount(serviceAccountName string) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name: serviceAccountName,
		},
	}
}

func newKiagnoseConfigMapAccessRole(configMapAccessRole string) *rbacv1.Role {
	return &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name: configMapAccessRole,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"configmaps"},
				Verbs:     []string{"get", "update"},
			},
		},
	}
}

func newKubeVirtDPDKCheckerRole() *rbacv1.Role {
	return &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name: testKubeVirtDPDKCheckerRoleName,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{"kubevirt.io"},
				Resources: []string{"virtualmachineinstances"},
				Verbs:     []string{"create", "get", "delete"},
			},
			{
				APIGroups: []string{"subresources.kubevirt.io"},
				Resources: []string{"virtualmachineinstances/console"},
				Verbs:     []string{"get"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"configmaps"},
				Verbs:     []string{"create", "delete"},
			},
		},
	}
}

func newRoleBinding(name, serviceAccountName, roleName string) *rbacv1.RoleBinding {
	return &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind: rbacv1.ServiceAccountKind,
				Name: serviceAccountName,
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "Role",
			Name:     roleName,
		},
	}
}

func newConfigMap() *corev1.ConfigMap {
	testConfig := map[string]string{
		"spec.timeout": testTimeout.String(),
		"spec.param.networkAttachmentDefinitionName": networkAttachmentDefinitionName,
		"spec.param.trafficGenPacketsPerSecond":      "8m",
		"spec.param.testDuration":                    "1m",
		"spec.param.verbose":                         "true",
	}

	if trafficGenContainerDiskImage != "" {
		testConfig["spec.param.trafficGenContainerDiskImage"] = trafficGenContainerDiskImage
	}

	if vmUnderTestContainerDiskImage != "" {
		testConfig["spec.param.vmUnderTestContainerDiskImage"] = vmUnderTestContainerDiskImage
	}

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: testConfigMapName,
		},
		Data: testConfig,
	}
}

func getJobConditions() ([]batchv1.JobCondition, error) {
	checkupJob, err := virtClient.BatchV1().Jobs(testNamespace).Get(context.Background(), testCheckupJobName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	return checkupJob.Status.Conditions, nil
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
							Image:           testCheckupImageName,
							ImagePullPolicy: corev1.PullAlways,
							SecurityContext: newSecurityContext(),
							Env: []corev1.EnvVar{
								{
									Name:  "CONFIGMAP_NAMESPACE",
									Value: testNamespace,
								},
								{
									Name:  "CONFIGMAP_NAME",
									Value: testConfigMapName,
								},
								{
									Name: "POD_UID",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "metadata.uid",
										},
									},
								},
							},
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
