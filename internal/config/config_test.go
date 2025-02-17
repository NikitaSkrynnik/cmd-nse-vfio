// Copyright (c) 2020-2021 Doc.ai and/or its affiliates.
//
// Copyright (c) 2023 Cisco and/or its affiliates.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at:
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package config_test

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/NikitaSkrynnik/cmd-nse-vfio/internal/config"
)

func TestServiceConfig_UnmarshalBinary(t *testing.T) {
	cfg := new(config.ServiceConfig)
	err := cfg.UnmarshalBinary([]byte("pingpong: { addr: 0a:55:44:33:22:11 }"))
	require.NoError(t, err)

	require.Equal(t, &config.ServiceConfig{
		Name:    "pingpong",
		MACAddr: net.HardwareAddr{0x0a, 0x55, 0x44, 0x33, 0x22, 0x11},
	}, cfg)

	cfg = new(config.ServiceConfig)
	err = cfg.UnmarshalBinary([]byte("pingpong: { vlan: 1111 }"))
	require.NoError(t, err)

	require.Equal(t, &config.ServiceConfig{
		Name:    "pingpong",
		VLANTag: 1111,
	}, cfg)

	cfg = new(config.ServiceConfig)
	err = cfg.UnmarshalBinary([]byte("pingpong"))
	require.NoError(t, err)

	require.Equal(t, &config.ServiceConfig{
		Name: "pingpong",
	}, cfg)
}
