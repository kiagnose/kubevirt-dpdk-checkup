#!/usr/bin/env bash
#
# This file is part of the kiagnose project
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#
# Copyright 2023 Red Hat, Inc.
#

set -eu

if [ "${SET_VERBOSE}" == "TRUE" ]; then
	set -x
fi

expand_number_list() {
	local input_list=$1
	expanded_input=""
	IFS=',' read -ra nums <<< "$input_list"
	for num in "${nums[@]}"; do
		if [[ $num == *"-"* ]]; then
			IFS='-' read -ra range <<< "$num"
			for i in $(seq "${range[0]}" "${range[1]}"); do
				expanded_input+=",$i"
			done
		else
			expanded_input+=",$num"
		fi
	done
	expanded_input=${expanded_input:1} # remove the leading comma
	echo "$expanded_input"
}

remove_number() {
	local list=$1
	local number=$2
	local new_list=""
	IFS=',' read -r -a array <<< "$list"
	for i in "${array[@]}"; do
		if [ "$i" != "$number" ]; then
			new_list="$new_list,$i"
		fi
	done
	new_list=${new_list:1} # remove the leading comma
	echo "${new_list}"
}

set_pci_addresses() {
	# set interfaces
	IFS=',' read -r -a nics_array <<< "${!PCI_DEVICES_VAR_NAME}"
	export PCIDEVICE_NIC_1="\"${nics_array[0]}\""
	export PCIDEVICE_NIC_2="\"${nics_array[1]}\""
}

get_numa_socket_by_nic() {
	local nic_without_quotes="${1//\"/}"
	local lspci_output=$(lspci -v -nn -mm -k -s "${nic_without_quotes}")
	local numa_socket=$(echo "$lspci_output" | grep NUMANode | awk '{print $2}')
	echo "${numa_socket}"
}

set_numa_socket() {
	local nic1_numa_socket=$(get_numa_socket_by_nic "${PCIDEVICE_NIC_1}")
	local nic2_numa_socket=$(get_numa_socket_by_nic "${PCIDEVICE_NIC_2}")

	if [[ "${nic1_numa_socket}" != "${nic2_numa_socket}" ]]; then
		echo "error - NUMA socket of NICs ${PCIDEVICE_NIC_1} (=${nic1_numa_socket}), ${PCIDEVICE_NIC_2} (=${nic2_numa_socket}) do not match"
		exit 1
	fi

	export NUMA_SOCKET="${nic1_numa_socket}"
}

set_cpu_configs() {
	# set master
	CPUS=$(cat /sys/fs/cgroup/cpuset/cpuset.cpus)
	CPUS=$(expand_number_list "${CPUS}")
	IFS=',' read -r -a cpus_array <<< "${CPUS}"
	export MASTER=${cpus_array[0]}

	# set latency
	SIBLINGS=$(cat /sys/devices/system/cpu/cpu"${MASTER}"/topology/core_cpus_list)
	export LATENCY=$(remove_number "$SIBLINGS" "${MASTER}")

	# set cpu
	CPU=${CPUS}
	CPU=$(remove_number "${CPU}" "${MASTER}")
	CPU=$(remove_number "${CPU}" "${LATENCY}")
	export CPU=$CPU
}

print_params() {
	echo setting params:
	echo PCIDEVICE_NIC_1="${PCIDEVICE_NIC_1}"
	echo PCIDEVICE_NIC_2="${PCIDEVICE_NIC_2}"
	echo MASTER="${MASTER}"
	echo LATENCY="${LATENCY}"
	echo NUMA_SOCKET="${NUMA_SOCKET}"
	echo CPU="${CPU}"
	echo PORT_BANDWIDTH_GB="${PORT_BANDWIDTH_GB}"
}

set_pci_addresses

set_numa_socket

set_cpu_configs

print_params

# set config_template file
envsubst < /opt/templates/trex_cfg.yaml.in > /etc/trex_cfg.yaml

cat /etc/trex_cfg.yaml
