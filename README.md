# kubevirt-dpdk-checkup

checkup validating DPDK readiness of cluster, using the Kiagnose engine

## Permissions

You need to be a cluster-admin in order to execute this checkup.
The checkup requires the following permissions:

```yaml
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: dpdk-checkup-sa
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: kiagnose-configmap-access
rules:
  - apiGroups: [ "" ]
    resources: [ "configmaps" ]
    verbs: [ "get", "update" ]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: kiagnose-configmap-access
subjects:
  - kind: ServiceAccount
    name: dpdk-checkup-sa
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: kiagnose-configmap-access
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: kubevirt-dpdk-checker
rules:
  - apiGroups: [ "kubevirt.io" ]
    resources: [ "virtualmachineinstances" ]
    verbs: [ "create", "get", "delete" ]
  - apiGroups: [ "subresources.kubevirt.io" ]
    resources: [ "virtualmachineinstances/console" ]
    verbs: [ "get" ]
  - apiGroups: [ "" ]
    resources: [ "pods" ]
    verbs: [ "create", "get", "delete" ]
  - apiGroups: [ "" ]
    resources: [ "pods/exec" ]
    verbs: [ "create" ]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: kubevirt-dpdk-checker
subjects:
  - kind: ServiceAccount
    name: dpdk-checkup-sa
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: kubevirt-dpdk-checker
```

## Configuration

| Key                                         | Description                                                                                                       | Is Mandatory | Remarks                                                                                                |
|---------------------------------------------|-------------------------------------------------------------------------------------------------------------------|--------------|--------------------------------------------------------------------------------------------------------|
| spec.timeout                                | How much time before the checkup will try to close itself                                                         | True         |                                                                                                        |
| spec.param.NUMASocket                       | The NUMA node where the workloads shall be scheduled to                                                           | True         |                                                                                                        |
| spec.param.networkAttachmentDefinitionName  | NetworkAttachmentDefinition name of the SR-IOV NICs connected                                                     | True         | Assumed to be in the same namespace                                                                    |
| spec.param.trafficGeneratorRuntimeClassName | [Runtime Class](https://kubernetes.io/docs/concepts/containers/runtime-class/) the traffic generator pod will use | True         |                                                                                                        |
| spec.param.trafficGeneratorImage            | Traffic generator's container image                                                                               | False        | Defaults to the U/S image https://quay.io/repository/kiagnose/kubevirt-dpdk-checkup-traffic-gen:latest |
| spec.param.trafficGeneratorNodeSelector     | Node Name on which the traffic generator Pod will be scheduled to                                                 | False        | Assumed to be configured to Nodes that allow DPDK traffic                                              |
| spec.param.trafficGeneratorPacketsPerSecond | Amount of packets per second in Millions                                                                          | False        | Defaults to 14                                                                                         |
| spec.param.trafficGeneratorEastMacAddress   | MAC address of the NIC connected to the traffic generator pod/VM                                                  | False        | Defaults to 50:00:00:00:00:01                                                                          |
| spec.param.trafficGeneratorWestMacAddress   | MAC address of the NIC connected to the traffic generator pod/VM                                                  | False        | Defaults to 50:00:00:00:00:02                                                                          |
| spec.param.DPDKLabelSelector                | Node Label of on which the VM shall run                                                                           | False        | Assumed to be configured to Nodes that allow DPDK traffic                                              |
| spec.param.DPDKEastMacAddress               | MAC address of the NIC connected to the DPDK VM                                                                   | False        | Defaults to 60:00:00:00:00:01                                                                          |
| spec.param.DPDKWestMacAddress               | MAC address of the NIC connected to the DPDK VM                                                                   | False        | Defaults to 60:00:00:00:00:02                                                                          |
| spec.param.testDuration                     | How much time will the traffic generator will run                                                                 | False        | Defaults to 5 Minutes                                                                                  |
| spec.param.portBandwidthGB                  | SR-IOV NIC max bandwidth                                                                                          | False        | Defaults to 10GB                                                                                       |

### Example

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: dpdk-checkup-config
data:
  spec.timeout: 10m
  spec.param.NUMASocket: 0
  spec.param.networkAttachmentDefinitionName: <network-name>
  spec.param.trafficGeneratorRuntimeClassName: <runtimeclass-name>
  spec.param.trafficGeneratorImage: quay.io/kiagnose/kubevirt-dpdk-checkup-traffic-gen:latest
```
