/*
Copyright 2020 The Kubernetes Authors.

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
	"path/filepath"
	"testing"
)

func TestSetRepoPathIfNotSet(t *testing.T) {
	tmpdir, err := filepath.EvalSymlinks(os.TempDir())
	if err != nil {
		t.Errorf("failed to read tempdir")
	}
	cases := []struct {
		name string

		initialDeployer  Deployer
		expectedRepoPath string
	}{
		{
			name:             "set empty repo path",
			expectedRepoPath: tmpdir,
		},
		{
			name: "set preset repo path",
			initialDeployer: Deployer{
				RepoRoot: "/test/path",
			},
			expectedRepoPath: "/test/path",
		},
	}

	err = os.Chdir(os.TempDir())
	if err != nil {
		t.Errorf("failed to chdir for test: %s", err)
	}

	for i := range cases {
		c := &cases[i]
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			d := &c.initialDeployer
			err := d.setRepoPathIfNotSet()
			if err != nil {
				t.Errorf("failed to set repo path: %s", err)
			}

			if d.RepoRoot != c.expectedRepoPath {
				t.Errorf("expected repo path to be %s but it was %s", c.expectedRepoPath, d.RepoRoot)
			}
		})
	}
}
