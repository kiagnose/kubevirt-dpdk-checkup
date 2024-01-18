# kubevirt-dpdk-checkup

checkup validating DPDK readiness of cluster, using the Kiagnose engine

## Permissions

You need to be a namespace-admin in order to execute this checkup.
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
    resources: [ "virtualmachineinstances/console", "virtualmachineinstances/softreboot" ]
    verbs: [ "get", "update" ]
  - apiGroups: [ "" ]
    resources: [ "configmaps" ]
    verbs: [ "create", "delete" ]
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

| Key                                        | Description                                                            | Is Mandatory | Remarks                                                               |
|--------------------------------------------|------------------------------------------------------------------------|--------------|-----------------------------------------------------------------------|
| spec.timeout                               | How much time before the checkup will try to close itself              | True         |                                                                       |
| spec.param.networkAttachmentDefinitionName | NetworkAttachmentDefinition name of the SR-IOV NICs connected          | True         | Assumed to be in the same namespace                                   |
| spec.param.trafficGenContainerDiskImage    | Traffic generator's container disk image                               | False        | Defaults to `quay.io/kiagnose/kubevirt-dpdk-checkup-traffic-gen:main` |
| spec.param.trafficGenTargetNodeName        | Node Name on which the traffic generator VM will be scheduled to       | False        | Assumed to be configured to Nodes that allow DPDK traffic             |
| spec.param.trafficGenPacketsPerSecond      | Amount of packets per second. format: <amount>[/k/m] k-kilo; m-million | False        | Defaults to 8m                                                        |
| spec.param.vmUnderTestContainerDiskImage   | VM under test container disk image                                     | False        | Defaults to `quay.io/kiagnose/kubevirt-dpdk-checkup-vm:main`          |
| spec.param.vmUnderTestTargetNodeName       | Node Name on which the VM under test will be scheduled to              | False        | Assumed to be configured to Nodes that allow DPDK traffic             |
| spec.param.testDuration                    | How much time will the traffic generator will run                      | False        | Defaults to 5 Minutes                                                 |
| spec.param.portBandwidthGbps               | SR-IOV NIC max bandwidth                                               | False        | Defaults to 10Gbps                                                    |
| spec.param.verbose                         | Increases checkup's log verbosity                                      | False        | "true" / "false". Defaults to "false"                                 |

### Example

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: dpdk-checkup-config
data:
  spec.timeout: 10m
  spec.param.networkAttachmentDefinitionName: <network-name>
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
          image: quay.io/kiagnose/kubevirt-dpdk-checkup:main
          imagePullPolicy: Always
          securityContext:
            allowPrivilegeEscalation: false
            capabilities:
              drop: [ "ALL" ]
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

| Key                                        | Description                                                            | Remarks  |
|--------------------------------------------|------------------------------------------------------------------------|----------|
| status.succeeded                           | Specifies if the checkup is successful (`true`) or not (`false`)       |          |
| status.failureReason                       | The reason for failure if the checkup fails                            |          |
| status.startTimestamp                      | The time when the checkup started                                      | RFC 3339 |
| status.completionTimestamp                 | The time when the checkup has completed                                | RFC 3339 |
| status.result.trafficGenSentPackets        | The number of packets sent from the traffic generator                  |          |
| status.result.trafficGenOutputErrorPackets | The number of error packets sent from the traffic generator            |          |
| status.result.trafficGenInputErrorPackets  | The number of error packets received by the traffic generator          |          |
| status.result.trafficGenActualNodeName     | The node on which the traffic generator VM was scheduled               |          |
| status.result.vmUnderTestActualNodeName    | The node on which the VM under test was scheduled                      |          |
| status.result.vmUnderTestReceivedPackets   | The number of packets received on the VM under test                    |          |
| status.result.vmUnderTestRxDroppedPackets  | The ingress traffic packets that were dropped by the DPDK application  |          |
| status.result.vmUnderTestTxDroppedPackets  | The egress traffic packets that were dropped from the DPDK application |          |
