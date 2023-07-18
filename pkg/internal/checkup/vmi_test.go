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

package checkup_test

import (
	"testing"

	assert "github.com/stretchr/testify/require"

	k8scorev1 "k8s.io/api/core/v1"
	k8smetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kiagnose/kubevirt-dpdk-checkup/pkg/internal/checkup"
	"github.com/kiagnose/kubevirt-dpdk-checkup/pkg/internal/checkup/vmi"
)

func TestAffinityCalculation(t *testing.T) {
	const ownerUID = "123"

	t.Run("When node affinity is expected", func(t *testing.T) {
		nodeName := "node01"

		actualAffinity := checkup.Affinity(nodeName, ownerUID)

		expectedAffinity := &k8scorev1.Affinity{
			NodeAffinity: &k8scorev1.NodeAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: &k8scorev1.NodeSelector{
					NodeSelectorTerms: []k8scorev1.NodeSelectorTerm{
						{
							MatchExpressions: []k8scorev1.NodeSelectorRequirement{
								{
									Key:      k8scorev1.LabelHostname,
									Operator: k8scorev1.NodeSelectorOpIn,
									Values:   []string{nodeName}},
							},
						},
					},
				},
			},
		}

		assert.Equal(t, expectedAffinity, actualAffinity)
	})

	t.Run("When pod anti-affinity is expected", func(t *testing.T) {
		var nodeName string

		actualAffinity := checkup.Affinity(nodeName, ownerUID)

		expectedAffinity := &k8scorev1.Affinity{
			PodAntiAffinity: &k8scorev1.PodAntiAffinity{
				PreferredDuringSchedulingIgnoredDuringExecution: []k8scorev1.WeightedPodAffinityTerm{
					{
						Weight: 1,
						PodAffinityTerm: k8scorev1.PodAffinityTerm{
							TopologyKey: k8scorev1.LabelHostname,
							LabelSelector: &k8smetav1.LabelSelector{
								MatchExpressions: []k8smetav1.LabelSelectorRequirement{
									{
										Operator: k8smetav1.LabelSelectorOpIn,
										Key:      vmi.DPDKCheckupUIDLabelKey,
										Values:   []string{ownerUID},
									},
								},
							},
						},
					},
				},
			},
		}

		assert.Equal(t, expectedAffinity, actualAffinity)
	})
}

func TestCloudInitString(t *testing.T) {
	actualString := checkup.CloudInit("user", "password")
	expectedString := `#cloud-config
user: user
password: password
chpasswd:
  expire: false`

	assert.Equal(t, expectedString, actualString)
}
