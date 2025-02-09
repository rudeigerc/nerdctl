/*
   Copyright The containerd Authors.

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

package composer

import (
	"context"
	"strings"
	"sync"

	"github.com/containerd/containerd"
	"github.com/containerd/nerdctl/pkg/formatter"
	"github.com/containerd/nerdctl/pkg/labels"
	"github.com/containerd/nerdctl/pkg/strutil"

	"github.com/sirupsen/logrus"
)

// RemoveOptions stores all option input from `nerdctl compose rm`
type RemoveOptions struct {
	Stop    bool
	Volumes bool
}

// Remove removes stopped containers in `services`.
func (c *Composer) Remove(ctx context.Context, opt RemoveOptions, services []string) error {
	serviceNames, err := c.ServiceNames(services...)
	if err != nil {
		return err
	}
	// reverse dependency order
	for _, svc := range strutil.ReverseStrSlice(serviceNames) {
		containers, err := c.Containers(ctx, svc)
		if err != nil {
			return err
		}
		if opt.Stop {
			// use default Options to stop service containers.
			if err := c.stopContainers(ctx, containers, StopOptions{}); err != nil {
				return err
			}
		}
		if err := c.removeContainers(ctx, containers, opt); err != nil {
			return err
		}
	}
	return nil
}

func (c *Composer) removeContainers(ctx context.Context, containers []containerd.Container, opt RemoveOptions) error {
	args := []string{"rm", "-f"}
	if opt.Volumes {
		args = append(args, "-v")
	}

	var rmWG sync.WaitGroup
	for _, container := range containers {
		container := container
		rmWG.Add(1)
		go func() {
			defer rmWG.Done()
			info, _ := container.Info(ctx, containerd.WithoutRefreshedMetadata)
			// if no `--stop` is passed, check status and skip running container
			if !opt.Stop {
				cStatus := formatter.ContainerStatus(ctx, container)
				if strings.HasPrefix(cStatus, "Up") {
					logrus.Warnf("Removing container %s failed: container still running.", info.Labels[labels.Name])
					return
				}
			}

			logrus.Infof("Removing container %s", info.Labels[labels.Name])
			if err := c.runNerdctlCmd(ctx, append(args, container.ID())...); err != nil {
				logrus.Warn(err)
			}
		}()
	}
	rmWG.Wait()

	return nil
}
