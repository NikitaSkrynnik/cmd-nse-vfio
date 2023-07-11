// Copyright (c) 2020-2022 Doc.ai and/or its affiliates.
//
// Copyright (c) 2020-2022 Cisco and/or its affiliates.
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

package main_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	nested "github.com/antonfisher/nested-logrus-formatter"
	"github.com/edwarnicke/exechelper"
	"github.com/edwarnicke/grpcfd"
	"github.com/sirupsen/logrus"
	"github.com/spiffe/go-spiffe/v2/spiffetls/tlsconfig"
	"github.com/spiffe/go-spiffe/v2/workloadapi"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/NikitaSkrynnik/api/pkg/api/registry"
	"github.com/NikitaSkrynnik/sdk/pkg/registry/common/begin"
	"github.com/NikitaSkrynnik/sdk/pkg/registry/common/expire"
	"github.com/NikitaSkrynnik/sdk/pkg/registry/common/memory"
	registryrecvfd "github.com/NikitaSkrynnik/sdk/pkg/registry/common/recvfd"
	"github.com/NikitaSkrynnik/sdk/pkg/registry/core/adapters"
	registrychain "github.com/NikitaSkrynnik/sdk/pkg/registry/core/chain"
	"github.com/NikitaSkrynnik/sdk/pkg/tools/log"
	"github.com/NikitaSkrynnik/sdk/pkg/tools/log/logruslogger"
	"github.com/NikitaSkrynnik/sdk/pkg/tools/spire"
)

func (f *TestSuite) SetupSuite() {
	logrus.SetFormatter(&nested.Formatter{})
	log.EnableTracing(true)
	f.ctx, f.cancel = context.WithCancel(context.Background())
	f.ctx = log.WithLog(f.ctx, logruslogger.New(f.ctx))

	starttime := time.Now()

	// Get config from env
	// ********************************************************************************
	log.FromContext(f.ctx).Infof("Getting Config from Env (time since start: %s)", time.Since(starttime))
	// ********************************************************************************
	f.Require().NoError(f.config.Process())

	// ********************************************************************************
	log.FromContext(f.ctx).Infof("Running Spire (time since start: %s)", time.Since(starttime))
	// ********************************************************************************
	executable, err := os.Executable()
	f.Require().NoError(err)
	f.spireErrCh = spire.Start(
		spire.WithContext(f.ctx),
		spire.WithEntry("spiffe://example.org/app", "unix:path:/bin/app"),
		spire.WithEntry(fmt.Sprintf("spiffe://example.org/%s", filepath.Base(executable)),
			fmt.Sprintf("unix:path:%s", executable),
		),
	)
	f.Require().Len(f.spireErrCh, 0)

	// ********************************************************************************
	log.FromContext(f.ctx).Infof("Getting X509Source (time since start: %s)", time.Since(starttime))
	// ********************************************************************************
	source, err := workloadapi.NewX509Source(f.ctx)
	f.x509source = source
	f.x509bundle = source
	f.Require().NoError(err)
	svid, err := f.x509source.GetX509SVID()
	f.Require().NoError(err, "error getting x509 svid")
	log.FromContext(f.ctx).Infof("SVID: %q received (time since start: %s)", svid.ID, time.Since(starttime))

	// ********************************************************************************
	log.FromContext(f.ctx).Infof("Running system under test (SUT) (time since start: %s)", time.Since(starttime))
	// ********************************************************************************
	cmdStr := "app"
	f.sutErrCh = exechelper.Start(cmdStr,
		exechelper.WithContext(f.ctx),
		exechelper.WithEnvirons(os.Environ()...),
		exechelper.WithStdout(os.Stdout),
		exechelper.WithStderr(os.Stderr),
		exechelper.WithGracePeriod(30*time.Second),
	)
	f.Require().Len(f.sutErrCh, 0)

	// ********************************************************************************
	log.FromContext(f.ctx).Infof("Creating registryServer and registryClient (time since start: %s)", time.Since(starttime))
	// ********************************************************************************
	memrg := memory.NewNetworkServiceEndpointRegistryServer()
	registryServer := registrychain.NewNetworkServiceEndpointRegistryServer(
		begin.NewNetworkServiceEndpointRegistryServer(),
		expire.NewNetworkServiceEndpointRegistryServer(f.ctx, expire.WithDefaultExpiration(24*time.Hour)),
		registryrecvfd.NewNetworkServiceEndpointRegistryServer(),
		memrg,
	)

	// ********************************************************************************
	log.FromContext(f.ctx).Infof("Get the regEndpoint from SUT (time since start: %s)", time.Since(starttime))
	// ********************************************************************************
	serverCreds := credentials.NewTLS(tlsconfig.MTLSServerConfig(f.x509source, f.x509bundle, tlsconfig.AuthorizeAny()))
	serverCreds = grpcfd.TransportCredentials(serverCreds)
	server := grpc.NewServer(grpc.Creds(serverCreds))

	registry.RegisterNetworkServiceEndpointRegistryServer(server, registryServer)
	registry.RegisterNetworkServiceRegistryServer(server, memory.NewNetworkServiceRegistryServer())
	ctx, cancel := context.WithCancel(f.ctx)
	defer func(cancel context.CancelFunc, serverErrCh <-chan error) {
		cancel()
		err = <-serverErrCh
		f.Require().NoError(err)
	}(cancel, f.ListenAndServe(ctx, server))

	recv, err := adapters.NetworkServiceEndpointServerToClient(memrg).Find(ctx, &registry.NetworkServiceEndpointQuery{
		NetworkServiceEndpoint: &registry.NetworkServiceEndpoint{
			Name: f.config.Name,
		},
		Watch: true,
	})
	f.Require().NoError(err)

	regEndpointResp, err := recv.Recv()
	f.Require().NoError(err)
	log.FromContext(f.ctx).Infof("Received regEndpoint: %+v (time since start: %s)", regEndpointResp.NetworkServiceEndpoint, time.Since(starttime))

	// ********************************************************************************
	log.FromContext(f.ctx).Infof("Creating grpc.ClientConn to SUT (time since start: %s)", time.Since(starttime))
	// ********************************************************************************
	clientCreds := credentials.NewTLS(tlsconfig.MTLSClientConfig(f.x509source, f.x509bundle, tlsconfig.AuthorizeAny()))
	clientCreds = grpcfd.TransportCredentials(clientCreds)
	f.sutCC, err = grpc.DialContext(f.ctx,
		regEndpointResp.NetworkServiceEndpoint.GetUrl(),
		grpc.WithTransportCredentials(clientCreds),
		grpc.WithBlock(),
	)
	f.Require().NoError(err)

	// ********************************************************************************
	log.FromContext(f.ctx).Infof("SetupSuite Complete (time since start: %s)", time.Since(starttime))
	// ********************************************************************************
}

func (f *TestSuite) TearDownSuite() {
	f.cancel()
	for {
		_, ok := <-f.sutErrCh
		if !ok {
			break
		}
	}
	for {
		_, ok := <-f.spireErrCh
		if !ok {
			break
		}
	}
}
