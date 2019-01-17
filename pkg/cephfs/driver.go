/*
Copyright 2018 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cephfs

import (
	"github.com/golang/glog"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/kubernetes-csi/drivers/pkg/csi-common"

	"github.com/ceph/ceph-csi/pkg/util"
)

const (
	PluginFolder = "/var/lib/kubelet/plugins/csi-cephfsplugin"
	Version      = "1.0.0"
)

type driver struct {
	cd *csicommon.CSIDriver

	is *identityServer
	ns *nodeServer
	cs *controllerServer
}

var (
	DefaultVolumeMounter string
)

func NewDriver() *driver {
	return &driver{}
}

func NewIdentityServer(d *csicommon.CSIDriver) *identityServer {
	return &identityServer{
		DefaultIdentityServer: csicommon.NewDefaultIdentityServer(d),
	}
}

func NewControllerServer(d *csicommon.CSIDriver, cachePersister util.CachePersister) *controllerServer {
	return &controllerServer{
		DefaultControllerServer: csicommon.NewDefaultControllerServer(d),
		MetadataStore:           cachePersister,
	}
}

func NewNodeServer(d *csicommon.CSIDriver) *nodeServer {
	return &nodeServer{
		DefaultNodeServer: csicommon.NewDefaultNodeServer(d),
	}
}

func (fs *driver) Run(driverName, nodeID, endpoint, volumeMounter string, cachePersister util.CachePersister) {
	glog.Infof("Driver: %v version: %v", driverName, Version)

	// Configuration

	if err := loadAvailableMounters(); err != nil {
		glog.Fatalf("cephfs: failed to load ceph mounters: %v", err)
	}

	if volumeMounter != "" {
		if err := validateMounter(volumeMounter); err != nil {
			glog.Fatalln(err)
		} else {
			DefaultVolumeMounter = volumeMounter
		}
	} else {
		// Pick the first available mounter as the default one.
		// The choice is biased towards "fuse" in case both
		// ceph fuse and kernel mounters are available.
		DefaultVolumeMounter = availableMounters[0]
	}

	glog.Infof("cephfs: setting default volume mounter to %s", DefaultVolumeMounter)

	// Initialize default library driver

	fs.cd = csicommon.NewCSIDriver(driverName, Version, nodeID)
	if fs.cd == nil {
		glog.Fatalln("Failed to initialize CSI driver")
	}

	fs.cd.AddControllerServiceCapabilities([]csi.ControllerServiceCapability_RPC_Type{
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
	})

	fs.cd.AddVolumeCapabilityAccessModes([]csi.VolumeCapability_AccessMode_Mode{
		csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
	})

	// Create gRPC servers

	fs.is = NewIdentityServer(fs.cd)
	fs.ns = NewNodeServer(fs.cd)

	fs.cs = NewControllerServer(fs.cd, cachePersister)

	server := csicommon.NewNonBlockingGRPCServer()
	server.Start(endpoint, fs.is, fs.cs, fs.ns)
	server.Wait()
}
