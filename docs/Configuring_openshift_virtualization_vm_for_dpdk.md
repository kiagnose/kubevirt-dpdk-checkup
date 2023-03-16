# Configuring OpenShift Virtualization VM for DPDK workloads

## Introduction

Running [DPDK](https://core.dpdk.org/doc/) workloads on VMs requires careful configuration steps, both on the cluster and guest levels. However, by following the guidelines provided in this article, users can achieve lower latency and higher throughput for packet processing in user space.

## Prerequisites

- [Deploy](https://www.redhat.com/en/technologies/cloud-computing/openshift/deploy-red-hat-openshift) an OpenShift Container Platform cluster (OCP v4.13)
- [Deploy](https://docs.openshift.com/container-platform/4.12/virt/install/installing-virt-cli.html) OpenShift Virtualization plugin.
- [Deploy](https://docs.openshift.com/container-platform/4.12/networking/hardware_networks/installing-sriov-operator.html) SR-IOV Network Operator.
- [Create](https://docs.openshift.com/container-platform/4.12/applications/projects/working-with-projects.html) a project on the cluster where the VM will run.
- The relevant worker nodes must have:
  - SR-IOV enabled NICs.
  - Allocatable Hugepages.

> **Note**: 
> In this article an [Intel X710](https://www.dell.com/en-us/shop/intel-x710-quad-port-10gb-direct-attach-sfp-converged-network-adapter-full-height/apd/540-bbiw/networking#tabs_section) Network Adapter was used.

## Configuration

The configuration chapter will consist of three parts:
- Cluster configuration - requires cluster-admin permissions.
- VM spec configuration - requires project-admin permissions.
- Guest configuration - based on RHEL8 OS.

### Cluster configurations

This step consists of:
- Deploying a `vfio-pci` typed [SriovNetworkNodePolicy](https://docs.openshift.com/container-platform/4.12/networking/hardware_networks/configuring-sriov-device.html) and [SriovNetwork](https://docs.openshift.com/container-platform/4.12/networking/hardware_networks/configuring-sriov-device.html#cnf-assigning-a-sriov-network-to-a-vrf_configuring-sriov-device) using the [sriov-network-operator](https://github.com/openshift/sriov-network-operator) that is deployed on the cluster.
Doing so will result in the creation of a [network-attachment-definition](https://docs.openshift.com/container-platform/4.12/virt/virtual_machines/vm_networking/virt-attaching-vm-multiple-networks.html) that will be used by the VM in the next steps.
- [Checking](https://docs.openshift.com/container-platform/4.12/post_installation_configuration/machine-configuration-tasks.html#checking-mco-status_post-install-machine-configuration-tasks) the node has a machineConfigPool, and [creating](https://access.redhat.com/solutions/5688941) one if needed.
- Creating a [performance-profile](https://docs.openshift.com/container-platform/4.12/scalability_and_performance/cnf-create-performance-profiles.html) using the [Node-Tuning-Operator](https://docs.openshift.com/container-platform/4.12/scalability_and_performance/using-node-tuning-operator.html).
Doing so will create a [runtimeClass](https://kubernetes.io/docs/concepts/containers/runtime-class/) that would be used by the VM in the next steps.

In order to configure the cluster according to these steps please follow this [OCP documentation](https://docs.openshift.com/container-platform/4.12/networking/hardware_networks/using-dpdk-and-rdma.html).

## VM configuration

### Kubevirt cluster wide configuration

The pod running the VM (a.k.a. the virt-launcher pod) needs to run the DPDK optimized performance profile defined in the previous chapter. This is done by selecting the runtimeclassName in the pod running the VM.
Since currently Kubevirt does not support setting the runtimeclassName per VM, the change needs to be on a cluster level, by using the [jsonpatch annotations](https://github.com/kubevirt/hyperconverged-cluster-operator/blob/main/docs/cluster-configuration.md#jsonpatch-annotations) on HCO:
```bash
oc annotate --overwrite -n openshift-cnv hco kubevirt-hyperconverged \
  kubevirt.kubevirt.io/jsonpatch='[{"op": "add", \
    "path": "/spec/configuration/DefaultRuntimeClassName", \
    "value": <runtimeclass-name>}]'
```
> **Note**: 
> After setting this conifiguration all VMs will be assigned to the default RuntimeClassName.

### VM configuration

When configuring the VM spec, the following changes need to be made.
The changes are explained on each subchapter with appropriate snippets. In the end, an example VM template is offered.

#### Requesting the SR-IOV interfaces

If for example the created SR-IOV [network-attachment-definition](https://docs.openshift.com/container-platform/4.12/virt/virtual_machines/vm_networking/virt-attaching-vm-multiple-networks.html) name is `dpdk-net`, then the VM template interface request snippet should be like this:
```yaml
apiVersion: kubevirt.io/v1
kind: VirtualMachine
spec:
  template:
    spec:
...
      domain:
        interfaces:
          - sriov: {}
            name: nic-east
            pciAddress: '0000:07:00.0'
...
      networks:
        - multus:
            networkName: dpdk-net
          name: nic-east
...
```
> **Note**
> In order to more easily identify and configure the SR-IOV interface inside the guest, NIC’s guest PCIAddress on the VM spec is set to 0000:07:00.0.

#### CPU pinning

To allow pinning a specific CPU quantity to the VM spec, the cpu section should look like this:
```yaml
apiVersion: kubevirt.io/v1
kind: VirtualMachine
spec:
  template:
    spec:
...
      domain:
        cpu:
          sockets: 1
          threads: 2
          cores: 5
          dedicatedCpuPlacement: true
...
```
> **Notes**
> - The sockets field should be 1, in order to make sure the CPUs are scheduled from the same NUMA Node.
> - In this snippet example, the VM will be scheduled with 5 [hyper-threads](https://access.redhat.com/articles/7445) (or 10 CPUs).

#### CRI-O related annotations

The following annotations are added, in order to ensure high performance on the [CRI-O](https://cri-o.io/) side. In order for these annotations to work, the VM underlying pod needs to run with the runtimeClassName configured on the previous chapters.
```yaml
apiVersion: kubevirt.io/v1
kind: VirtualMachine
spec:
  template:
    metadata:
      annotations:
        cpu-load-balancing.crio.io: "disable"
        cpu-quota.crio.io: "disable"
        irq-load-balancing.crio.io: "disable"
...
```
> **Notes**
> - `cpu-load-balancing.crio.io: "disable"` - indicates that load balancing should be disabled for CPUs used by the container.
> - `cpu-quota.crio.io: "disable"` - indicates that CPU quota should be disabled for CPUs used by the container.
> - `irq-load-balancing.crio.io: "disable"` - indicates that IRQ load balancing should be disabled for CPUs used by the container.
> - These annotations will be copied to the Pod running the VM, and along with the runtimeClassName set by Kubevirt, will configure the VM for DPDK performance.

#### Hugepages request

In order to request an amount of [Hugepages](https://docs.openshift.com/container-platform/4.12/scalability_and_performance/what-huge-pages-do-and-how-they-are-consumed-by-apps.html), the resources snippet should look like this:
```yaml
apiVersion: kubevirt.io/v1
kind: VirtualMachine
spec:
  running: true
  template:
    spec:
...
      domain:
        resources:
          requests:
            memory: 8Gi
        memory:
          hugepages:
            pageSize: "1Gi"
...

```
> **Note** 
> In this snippet we request eight 1Gi sized Hugepages.

#### VM template summary example

In the end the VM template should look like this:
```yaml
apiVersion: kubevirt.io/v1
kind: VirtualMachine
metadata:
  name: rhel-dpdk-vm
spec:
  dataVolumeTemplates:
    - apiVersion: cdi.kubevirt.io/v1beta1
      kind: DataVolume
      metadata:
        creationTimestamp: null
        name: rhel-dpdk-vm
      spec:
        sourceRef:
          kind: DataSource
          name: rhel8
          namespace: openshift-virtualization-os-images
        storage:
          resources:
            requests:
              storage: 30Gi
  running: true
  template:
    metadata:
      annotations:
        cpu-load-balancing.crio.io: disable
        cpu-quota.crio.io: disable
        irq-load-balancing.crio.io: disable
    spec:
      domain:
        cpu:
          sockets: 1
          cores: 5
          threads: 2
          dedicatedCpuPlacement: true
        devices:
          disks:
            - disk:
                bus: virtio
              name: rootdisk
            - disk:
                bus: virtio
              name: cloudinitdisk
          interfaces:
            - masquerade: {}
              name: default
            - model: virtio
              name: nic-east
              pciAddress: '0000:07:00.0'
              sriov: {}
          networkInterfaceMultiqueue: true
          rng: {}
        memory:
          hugepages:
            pageSize: 1Gi
        resources:
          requests:
            memory: 8Gi
      networks:
        - name: default
          pod: {}
        - multus:
            networkName: dpdk-net
          name: nic-east
      terminationGracePeriodSeconds: 180
      volumes:
        - dataVolume:
            name: rhel-dpdk-vm
          name: rootdisk
        - cloudInitNoCloud:
            userData: |-
              #cloud-config
              user: cloud-user
              password: redhat
              chpasswd: { expire: False }
          name: cloudinitdisk
```

## Guest Configuration

Once the VM is deployed and running, there are configurations that need to be set once on the guest. The changes are explained on each subchapter with appropriate snippets. In the end, an example guest configuration summary script is offered.

### CPU isolation on guest

If we continue with the example from previous chapters, 5 hyper-threads translate to 10 CPUs on the guest:
```bash
cat /sys/fs/cgroup/cpuset/cpuset.cpus
```
The expected result should look like this:
```bash
0-9
```

If you have housekeeping processes running on the guest then at least 1 hyper thread should be left as non-isolated. Assuming this is true, the first CPU and its sibling should not be isolated:
```bash
cat /sys/devices/system/cpu/cpu0/topology/core_cpus_list
```
The expected result should look like this:
```bash
0-1
```

That leaves CPUS `2-9` to be configured as isolated.
Linux grub cmdLine on guest
The hugepages and isolated CPU are specified on the grub cmdLine:
```bash
grubby --update-kernel=ALL --args="default_hugepagesz=1GB hugepagesz=1G hugepages=8 isolcpus=2-9"
```

### CPU partitioning on guest

[tuned-adm](https://access.redhat.com/documentation/en-us/red_hat_enterprise_linux/7/html/performance_tuning_guide/sect-red_hat_enterprise_linux-performance_tuning_guide-tool_reference-tuned_adm) tool is used in order to set the cpu-partitioning performance profile.
```bash
dnf install -y tuned-profiles-cpu-partitioning
echo isolated_cores=2-9 > /etc/tuned/cpu-partitioning-variables.conf
tuned-adm profile cpu-partitioning
```

### Override the driver NIC on guest

The SR-IOV NIC driver on the guest needs to be overridden to be used as `vfio-pci`. This is done using the [driverctl](https://www.mankier.com/8/driverctl) tool
```bash
dnf install -y driverctl
driverctl set-override 0000:07:00.0 vfio-pci
```

### Guest configuration summary

The configurations that needs to be set is:
```bash
sudo su
dnf install -y driverctl,tuned-profiles-cpu-partitioning
grubby --update-kernel=ALL --args="default_hugepagesz=1GB hugepagesz=1G hugepages=8 isolcpus=2-9"
echo isolated_cores=2-9 > /etc/tuned/cpu-partitioning-variables.conf
tuned-adm profile cpu-partitioning
driverctl set-override 0000:07:00.0 vfio-pci

reboot
```

## Additional resources

- [OCP Low latency tuning documentation](https://docs.openshift.com/container-platform/4.12/scalability_and_performance/cnf-low-latency-tuning.html#node-tuning-operator-disabling-cpu-load-balancing-for-dpdk_cnf-master).
- [Validating DPDK performance on OpenShift](https://access.redhat.com/articles/6969629)
