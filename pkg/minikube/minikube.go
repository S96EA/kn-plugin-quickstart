// Copyright © 2021 The Knative Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package minikube

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"knative.dev/kn-plugin-quickstart/pkg/install"
)

var clusterName string
var kubernetesVersion = "1.22.4"
var minikubeVersion = 1.23

// SetUp creates a local Minikube cluster and installs all the relevant Knative components
func SetUp(name string) error {
	start := time.Now()
	clusterName = name

	if err := createMinikubeCluster(); err != nil {
		return fmt.Errorf("creating cluster: %w", err)
	}
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		fmt.Print("\n")
		fmt.Println("To finish setting up networking for minikube, run the following command in a separate terminal window:")
		fmt.Println("    minikube tunnel --profile knative")
		fmt.Println("The tunnel window must remain open until you are done with the quickstart environment.")
		fmt.Println("\nPress the Enter key to continue")
		fmt.Scanln()
	}
	if err := install.Serving(); err != nil {
		return fmt.Errorf("install serving: %w", err)
	}
	if err := install.Kourier(); err != nil {
		return fmt.Errorf("install kourier: %w", err)
	}
	if err := install.KourierMinikube(); err != nil {
		return fmt.Errorf("configure kourier: %w", err)
	}
	if err := install.Eventing(); err != nil {
		return fmt.Errorf("install eventing: %w", err)
	}

	finish := time.Since(start).Round(time.Second)
	fmt.Printf("🚀 Knative install took: %s \n", finish)
	fmt.Println("🎉 Now have some fun with Serverless and Event Driven Apps!")

	return nil
}

func createMinikubeCluster() error {
	if err := checkMinikubeVersion(); err != nil {
		return fmt.Errorf("minikube version: %w", err)
	}
	if err := checkForExistingCluster(); err != nil {
		return fmt.Errorf("existing cluster: %w", err)
	}
	return nil
}

// checkMinikubeVersion validates that the user has the correct version of Minikube installed.
// If not, it prompts the user to download a newer version before continuing.
func checkMinikubeVersion() error {
	versionCheck := exec.Command("minikube", "version", "--short")
	out, err := versionCheck.CombinedOutput()
	if err != nil {
		return fmt.Errorf("minikube version: %w", err)
	}
	fmt.Printf("Minikube version is: %s\n", string(out))

	userMinikubeVersion, err := parseMinikubeVersion(string(out))
	if err != nil {
		return fmt.Errorf("parsing minikube version: %w", err)
	}
	if userMinikubeVersion < minikubeVersion {
		var resp string
		fmt.Printf("WARNING: We require at least Minikube v%.2f, while you are using v%.2f\n", minikubeVersion, userMinikubeVersion)
		fmt.Println("You can download a newer version from https://github.com/kubernetes/minikube/releases/")
		fmt.Print("Continue anyway? (not recommended) [y/N]: ")
		fmt.Scanf("%s", &resp)
		if strings.ToLower(resp) != "y" {
			fmt.Println("Installation stopped. Please upgrade minikube and run again")
			os.Exit(0)
		}
	}

	return nil
}

// checkForExistingCluster checks if the user already has a Minikube cluster. If so, it provides
// the option of deleting the existing cluster and recreating it. If not, it proceeds to
// creating a new cluster
func checkForExistingCluster() error {
	getClusters := exec.Command("minikube", "profile", "list")
	out, err := getClusters.CombinedOutput()
	if err != nil {
		// there are no existing minikube profiles, the listing profiles command will error
		// if there were no profiles, we simply want to create a new one and not stop the install
		// so if the error is the "MK_USAGE_NO_PROFILE" error, we ignore it and continue onwards
		if !strings.Contains(string(out), "MK_USAGE_NO_PROFILE") {
			return fmt.Errorf("check cluster: %w", err)
		}
	}
	// TODO Add tests for regex
	r := regexp.MustCompile(clusterName)
	matches := r.Match(out)
	if matches {
		var resp string
		fmt.Print("Knative Cluster " + clusterName + " already installed.\nDelete and recreate [y/N]: ")
		fmt.Scanf("%s", &resp)
		if strings.ToLower(resp) != "y" {
			fmt.Println("Installation skipped")
			return nil
		}
		fmt.Println("deleting cluster...")
		deleteCluster := exec.Command("minikube", "delete", "--profile", clusterName)
		if err := deleteCluster.Run(); err != nil {
			return fmt.Errorf("delete cluster: %w", err)
		}
		if err := createNewCluster(); err != nil {
			return fmt.Errorf("new cluster: %w", err)
		}
		return nil
	}

	if err := createNewCluster(); err != nil {
		return fmt.Errorf("new cluster: %w", err)
	}

	return nil
}

// createNewCluster creates a new Minikube cluster
func createNewCluster() error {
	fmt.Println("☸ Creating Minikube cluster...")
	fmt.Println("\nBy default, using the standard minikube driver for your system")
	fmt.Println("If you wish to use a different driver, please configure minikube using")
	fmt.Print("    minikube config set driver <your-driver>\n\n")

	// create cluster and wait until ready
	createCluster := exec.Command("minikube", "start", "--kubernetes-version", kubernetesVersion, "--cpus", "3", "--profile", clusterName, "--wait", "all", "--driver", "docker", "--image-mirror-country", "cn", "--registry-mirror", "https://x2og3451.mirror.aliyuncs.com", "--base-image", "registry.cn-hangzhou.aliyuncs.com/google_containers/kicbase:v0.0.28")
	if err := runCommandWithOutput(createCluster); err != nil {
		return fmt.Errorf("minikube create: %w", err)
	}

	return nil
}

func runCommandWithOutput(c *exec.Cmd) error {
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("piping output: %w", err)
	}
	fmt.Print("\n")
	return nil
}

func parseMinikubeVersion(v string) (float64, error) {
	strippedVersion := strings.TrimLeft(strings.TrimRight(v, "\n"), "v")
	dotVersion := strings.Split(strippedVersion, ".")
	floatVersion, err := strconv.ParseFloat(dotVersion[0]+"."+dotVersion[1], 64)
	if err != nil {
		return 0, err
	}

	return floatVersion, nil
}
