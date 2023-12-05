/*
Copyright 2021 The Kubernetes Authors.

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

package deployer

import (
	"os"

	"k8s.io/klog/v2"

	"sigs.k8s.io/kubetest2/pkg/build"
	"sigs.k8s.io/kubetest2/pkg/process"
)

func (d *Deployer) Build() error {
	args := []string{
		"build", "node-image",
	}
	if d.BuildType != "" {
		args = append(args, "--type", d.BuildType)
	}
	if d.KubeRoot != "" {
		args = append(args, "--kube-root", d.KubeRoot)
	}
	// set the explicitly specified image name if set
	if d.NodeImage != "" {
		args = append(args, "--image", d.NodeImage)
	} else if d.commonOptions.ShouldBuild() {
		// otherwise if we just built an image, use that
		args = append(args, "--image", kindDefaultBuiltImageName)
	}

	klog.V(0).Infof("Build(): building kind node image...\n")
	// we want to see the output so use process.ExecJUnit
	if err := process.ExecJUnit("kind", args, os.Environ()); err != nil {
		return err
	}
	build.StoreCommonBinaries(d.KubeRoot, d.commonOptions.RunDir())
	return nil
}
