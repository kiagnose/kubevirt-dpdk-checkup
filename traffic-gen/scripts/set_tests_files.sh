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

mkdir -p /opt/tests

print_params() {
	echo SRC_WEST_MAC_ADDRESS="${SRC_WEST_MAC_ADDRESS}"
	echo SRC_EAST_MAC_ADDRESS="${SRC_EAST_MAC_ADDRESS}"
	echo DST_WEST_MAC_ADDRESS="${DST_WEST_MAC_ADDRESS}"
	echo DST_EAST_MAC_ADDRESS="${DST_EAST_MAC_ADDRESS}"
	echo NUM_OF_CPUS="${NUM_OF_CPUS}"
}

print_params

# set tests files
envsubst < /opt/templates/testpmd.py.in > /opt/tests/testpmd.py
envsubst < /opt/templates/testpmd_addr.py.in > /opt/tests/testpmd_addr.py

cat /opt/tests/testpmd.py
cat /opt/tests/testpmd_addr.py
