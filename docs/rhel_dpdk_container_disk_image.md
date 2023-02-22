# Building RHEL DPDK VM ContainerDisk Image

Create a new RHEL 8.7 virtual machine, with at least the following specifications:

- 2 CPU Cores
- 4 GiB RAM
- 20 GB of free space in `/var`

In order to build the ContainerDisk image, the solution uses a pipeline of:

- [image-builder](https://access.redhat.com/documentation/en-us/red_hat_enterprise_linux/8/html-single/composing_a_customized_rhel_system_image/index)
- [virt-customize](https://www.libguestfs.org/virt-customize.1.html)
- [podman](https://podman.io)

All the following commands should be done in the VM's terminal.

The builder VM should be subscribed:

```bash
sudo subscription-manager register
```

> **Note:**
> For more details, please refer to https://access.redhat.com/solutions/253273

## Dependencies Installation

```bash
sudo dnf install libguestfs-tools
sudo dnf install podman
sudo dnf install osbuild-composer composer-cli bash-completion
sudo systemctl enable --now osbuild-composer.socket
source /etc/bash_completion.d/composer-cli
```

Add the user to the `weldr` group in order to use `composer-cli` without `sudo`:

```bash
newgrp weldr
sudo usermod -a -G weldr <user>
```

Verify that the image builder is working:

```bash
composer-cli status show
```

Verify that you can use RHEL 8.7 distro:

```bash
composer-cli distros list
```

## Base Image Build

Create the blueprint file:

```bash
cat << EOF > dpdk-vm.toml
name = "dpdk_image"
description = "Image to use with the DPDK checkup"
version = "0.0.1"
distro = "rhel-87"

[[packages]]
name = "dpdk"

[[packages]]
name = "dpdk-tools"

[[packages]]
name = "driverctl"

[[packages]]
name = "tuned-profiles-cpu-partitioning"

[customizations.kernel]
append = "default_hugepagesz=1GB hugepagesz=1G hugepages=8 isolcpus=2-7"

[customizations.services]
disabled = ["NetworkManager-wait-online", "sshd"]
EOF
```

Push the blueprint file:

```bash
composer-cli blueprints push dpdk-vm.toml
```

Start building the image:

```bash
composer-cli compose start dpdk_image qcow2
```

Periodically check for the build status, and wait for it to be `FINISHED`:

```bash
composer-cli compose list
```

Get the ready qcow2 image:

```bash
composer-cli compose image <UID>
```

## Base Image Customization

Create the customization scripts:

```bash
cat <<EOF >customize-vm
echo  isolated_cores=2-7 > /etc/tuned/cpu-partitioning-variables.conf
tuned-adm profile cpu-partitioning
echo "options vfio enable_unsafe_noiommu_mode=1" > /etc/modprobe.d/vfio-noiommu.conf
EOF

```

```bash
cat <<EOF >first-boot
driverctl set-override 0000:06:00.0 vfio-pci
driverctl set-override 0000:07:00.0 vfio-pci

mkdir /mnt/huge
mount /mnt/huge --source nodev -t hugetlbfs -o pagesize=1GB
EOF
```

Execute `virt-customize`:

```bash
virt-customize -a <UID>.qcow2 --run=customize-vm --firstboot=first-boot --selinux-relabel
```

## ContainerDisk Image Build

```bash
cat << EOF > Dockerfile
FROM scratch
COPY <uid>-disk.qcow2 /disk/
EOF
```

```bash
sudo podman build . -t dpdk-rhel:latest
```

Push the ContainerDisk image to a registry that is accessible to your cluster.

Provide a link to this image at the checkup’s configuration:

```
spec.param.vmContainerDiskImage: /path/to/this/image:<tag>
```
