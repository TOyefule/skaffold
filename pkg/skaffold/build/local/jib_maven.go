/*
Copyright 2018 The Skaffold Authors

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

package local

import (
	"context"
	"io"
	"strings"

	"fmt"

	"github.com/GoogleContainerTools/skaffold/pkg/skaffold/jib"
	"github.com/GoogleContainerTools/skaffold/pkg/skaffold/schema/latest"
	"github.com/GoogleContainerTools/skaffold/pkg/skaffold/util"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

func (b *Builder) buildJibMavenToDocker(ctx context.Context, out io.Writer, workspace string, a *latest.JibMavenArtifact) (string, error) {
	if a.Module != "" {
		// We require multimodule builds to explicitly bind a single jib goal to the `package` phase
		if err := hasSingleJibPackageGoal(ctx, workspace, a); err != nil {
			return "", err
		}
	}

	skaffoldImage := generateJibImageRef(workspace, a.Module)
	var args []string
	if a.Module != "" {
		args = generateMultiModuleMavenArgs(skaffoldImage, a)
	} else {
		args = generateSingleModuleMavenArgs("dockerBuild", skaffoldImage, a)
	}

	if err := runMavenCommand(ctx, out, workspace, args); err != nil {
		return "", err
	}

	return skaffoldImage, nil
}

func (b *Builder) buildJibMavenToRegistry(ctx context.Context, out io.Writer, workspace string, artifact *latest.Artifact) (string, error) {
	if artifact.JibMavenArtifact.Module != "" {
		return "", errors.New("maven multi-modules not supported yet")
	}

	initialTag := util.RandomID()
	skaffoldImage := fmt.Sprintf("%s:%s", artifact.ImageName, initialTag)
	args := generateSingleModuleMavenArgs("build", skaffoldImage, artifact.JibMavenArtifact)

	if err := runMavenCommand(ctx, out, workspace, args); err != nil {
		return "", err
	}

	return skaffoldImage, nil
}

// generateSingleModuleMavenArgs generates the arguments to Maven for building the project as an image called `skaffoldImage`.
func generateSingleModuleMavenArgs(goal string, skaffoldImage string, a *latest.JibMavenArtifact) []string {
	command := []string{"--non-recursive", "prepare-package", "jib:" + goal, "-Dimage=" + skaffoldImage}
	if a.Profile != "" {
		command = append(command, "-P"+a.Profile)
	}

	return command
}

// generateMultiModuleMavenArgs generates the arguments to Maven for building the project as an image called `skaffoldImage`.
func generateMultiModuleMavenArgs(skaffoldImage string, a *latest.JibMavenArtifact) []string {
	command := []string{"--projects", a.Module, "--also-make", "package", "-Dimage=" + skaffoldImage}
	if a.Profile != "" {
		command = append(command, "-P"+a.Profile)
	}

	return command
}

// hasSingleJibPackageGoal checks if the module has a single jib goal bound to
// `package`.  It returns `nil` if there is a single package goal. Otherwise
// it returns an error object.
func hasSingleJibPackageGoal(ctx context.Context, workspace string, a *latest.JibMavenArtifact) error {
	// cannot use --non-recursive
	command := []string{"--projects",a.Module,"jib:_skaffold-package-goals","--quiet"}
	if a.Profile != "" {
		command = append(command, "-P"+a.Profile)
	}

	cmd := jib.MavenCommand.CreateCommand(ctx, workspace, command)
	logrus.Debugf("Looking for jib bound package goals for %s: %s, %v", workspace, cmd.Path, cmd.Args)
	stdout, err := util.RunCmdOut(cmd)
	if err != nil {
		return errors.Wrap(err, "could not obtain jib package goals")
	}
	// need to trim last newline
	goals := strings.Split(strings.TrimSpace(string(stdout)), "\n")
	logrus.Debugf("jib bound package goals for %s %s: %v (%d)", workspace, a.Module, goals, len(goals))
	if len(goals) == 1 {
		return nil
	}
	return errors.New("skaffold requires a single jib goal bound to 'package'")
}

func runMavenCommand(ctx context.Context, out io.Writer, workspace string, args []string) error {
	cmd := jib.MavenCommand.CreateCommand(ctx, workspace, args)
	cmd.Stdout = out
	cmd.Stderr = out

	logrus.Infof("Building %s: %s, %v", workspace, cmd.Path, cmd.Args)
	if err := util.RunCmd(cmd); err != nil {
		return errors.Wrap(err, "maven build failed")
	}

	return nil
}
