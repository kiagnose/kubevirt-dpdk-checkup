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

echo "setting params to trex_cfg.yaml"
/opt/scripts/set_traffic_gen_cfg_file.sh

echo "setting params to traffic-gen test files"
/opt/scripts/set_tests_files.sh

echo "run traffic_gen daemon"
/opt/scripts/run_traffic_gen_daemon.sh
