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
  - apiGroups: [ "k8s.cni.cncf.io" ]
    resources: [ "network-attachment-definitions" ]
    verbs: [ "get" ]
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
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: dpdk-checkup-traffic-gen-sa
```

## Configuration

| Key                                         | Description                                                                                                       | Is Mandatory | Remarks                                                                                                |
|---------------------------------------------|-------------------------------------------------------------------------------------------------------------------|--------------|--------------------------------------------------------------------------------------------------------|
| spec.timeout                                | How much time before the checkup will try to close itself                                                         | True         |                                                                                                        |
| spec.param.networkAttachmentDefinitionName  | NetworkAttachmentDefinition name of the SR-IOV NICs connected                                                     | True         | Assumed to be in the same namespace                                                                    |
| spec.param.trafficGeneratorRuntimeClassName | [Runtime Class](https://kubernetes.io/docs/concepts/containers/runtime-class/) the traffic generator pod will use | True         |                                                                                                        |
| spec.param.trafficGeneratorImage            | Traffic generator's container image                                                                               | False        | Defaults to the U/S image https://quay.io/repository/kiagnose/kubevirt-dpdk-checkup-traffic-gen:latest |
| spec.param.trafficGeneratorNodeSelector     | Node Name on which the traffic generator Pod will be scheduled to                                                 | False        | Assumed to be configured to Nodes that allow DPDK traffic                                              |
| spec.param.trafficGeneratorPacketsPerSecond | Amount of packets per second. format: <amount>[/k/m] k-kilo; m-million                                            | False        | Defaults to 14m                                                                                        |
| spec.param.trafficGeneratorEastMacAddress   | MAC address of the NIC connected to the traffic generator pod/VM                                                  | False        | Defaults to a random MAC address of the form: 50:xx:xx:xx:xx:01                                        |
| spec.param.trafficGeneratorWestMacAddress   | MAC address of the NIC connected to the traffic generator pod/VM                                                  | False        | Defaults to a random MAC address of the form: 50:xx:xx:xx:xx:02                                        |
| spec.param.vmContainerDiskImage             | Container disk image for the VM                                                                                   | False        | Defaults to https://quay.io/repository/kiagnose/kubevirt-dpdk-checkup-vm:latest                        |
| spec.param.DPDKLabelSelector                | Node Label of on which the VM shall run                                                                           | False        | Assumed to be configured to Nodes that allow DPDK traffic                                              |
| spec.param.DPDKEastMacAddress               | MAC address of the NIC connected to the DPDK VM                                                                   | False        | Defaults to a random MAC address of the form: 60:xx:xx:xx:xx:01                                        |
| spec.param.DPDKWestMacAddress               | MAC address of the NIC connected to the DPDK VM                                                                   | False        | Defaults to a random MAC address of the form: 60:xx:xx:xx:xx:02                                        |
| spec.param.testDuration                     | How much time will the traffic generator will run                                                                 | False        | Defaults to 5 Minutes                                                                                  |
| spec.param.portBandwidthGB                  | SR-IOV NIC max bandwidth                                                                                          | False        | Defaults to 10GB                                                                                       |
| spec.param.verbose                          | Increases checkup's log verbosity                                                                                 | False        | "true" / "false". Defaults to "false"                                                                  |

### Example

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: dpdk-checkup-config
data:
  spec.timeout: 10m
  spec.param.networkAttachmentDefinitionName: <network-name>
  spec.param.trafficGeneratorRuntimeClassName: <runtimeclass-name>
```

## Execution
In order to execute the checkup, fill in the required data and apply this manifest:

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: dpdk-checkup
spec:
  backoffLimit: 0
  template:
    spec:
      serviceAccountName: dpdk-checkup-sa
      restartPolicy: Never
      containers:
        - name: dpdk-checkup
          image: quay.io/kiagnose/kubevirt-dpdk-checkup:latest
          imagePullPolicy: Always
          securityContext:
            allowPrivilegeEscalation: false
            capabilities:
              drop: ["ALL"]
            runAsNonRoot: true
            seccompProfile:
              type: "RuntimeDefault"
          env:
            - name: CONFIGMAP_NAMESPACE
              value: <target-namespace>
            - name: CONFIGMAP_NAME
              value: dpdk-checkup-config
            - name: POD_UID
              valueFrom:
                fieldRef:
                  fieldPath: metadata.uid
```

## Checkup Results Retrieval

After the checkup Job had completed, the results are made available at the user-supplied ConfigMap object:

```bash
kubectl get configmap dpdk-checkup-config -n <target-namespace> -o yaml
```


| Key                                              | Description                                                       | Remarks  |
|--------------------------------------------------|-------------------------------------------------------------------|----------|
| status.succeeded                                 | Has the checkup succeeded                                         |          |
| status.failureReason                             | Failure reason in case of a failure                               |          |
| status.startTimestamp                            | Checkup start timestamp                                           | RFC 3339 |
| status.completionTimestamp                       | Checkup completion timestamp                                      | RFC 3339 |
| status.result.trafficGeneratorTxPackets          | Number of packets sent from the traffic generator                 |          |
| status.result.trafficGeneratorOutputErrorPackets | Indicates error sending packets from traffic generator            |          |
| status.result.trafficGeneratorInErrorPackets     | Indicates error receiving packets to traffic generator            |          |
| status.result.trafficGeneratorNode               | Node name on which the traffic generator Pod was scheduled        |          |
| status.result.DPDKVMNode                         | Node name on which the DPDK VMI was scheduled                     |          |
| status.result.DPDKRxTestPackets                  | Number of packets packets received on the DPDK VMI                |          |
| status.result.DPDKRxPacketDrops                  | Indicates ingress packets that were dropped from DPDK application |          |
| status.result.DPDKTxPacketDrops                  | Indicates egress packets were dropped from the DPDK application   |          |
