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

package pod

import (
	"encoding/json"

	corev1 "k8s.io/api/core/v1"

	networkv1 "github.com/k8snetworkplumbingwg/network-attachment-definition-client/pkg/apis/k8s.cni.cncf.io/v1"
)

func CreateNetworksRequest(networkSelectionElementList []networkv1.NetworkSelectionElement) (string, error) {
	if len(networkSelectionElementList) == 0 {
		return "[]", nil
	}

	annotationValue, err := json.Marshal(networkSelectionElementList)
	if err != nil {
		return "", err
	}

	return string(annotationValue), nil
}

func WithNetworkRequestAnnotation(networkRequestAnnotationValue string) PodOption {
	return func(pod *corev1.Pod) {
		if pod.ObjectMeta.Annotations == nil {
			pod.ObjectMeta.Annotations = map[string]string{}
		}

		pod.ObjectMeta.Annotations[networkv1.NetworkAttachmentAnnot] = networkRequestAnnotationValue
	}
}
