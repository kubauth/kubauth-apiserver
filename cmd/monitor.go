/*
Copyright 2025 Kubotal

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

package cmd

import (
	"context"
	"fmt"
	"kubauth-apiserver/internal/config"
	"kubauth-apiserver/internal/global"
	"kubauth-apiserver/internal/k8sapi"
	"kubauth-apiserver/internal/misc"
	"kubauth-apiserver/internal/texttemplate"
	"log/slog"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/spf13/cobra"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes"
)

var monitorParams struct {
	logConfig        misc.LogConfig
	kubeconfig       string
	configFile       string
	remove           bool
	mark             bool
	force            bool
	ttlAfterFinished time.Duration
	jobTemplate      string
}

func init() {
	monitorCmd.PersistentFlags().StringVar(&monitorParams.logConfig.Level, "logLevel", "INFO", "Log level")
	monitorCmd.PersistentFlags().StringVar(&monitorParams.logConfig.Mode, "logMode", "text", "Log mode: 'text' or 'json'")
	monitorCmd.PersistentFlags().StringVar(&monitorParams.kubeconfig, "kubeconfig", "", "kubeconfig file path. Override default configuration.")
	monitorCmd.PersistentFlags().StringVar(&monitorParams.configFile, "configFile", "/config.yaml", "Configuration file")
	monitorCmd.PersistentFlags().StringVar(&monitorParams.jobTemplate, "jobTemplate", "/templates/job.tmpl", "Job template file for each node")
	monitorCmd.PersistentFlags().BoolVar(&monitorParams.remove, "remove", false, "Remove oidc configuration")
	monitorCmd.PersistentFlags().BoolVar(&monitorParams.force, "force", false, "Perform even if apiserver is down")
	monitorCmd.PersistentFlags().BoolVar(&monitorParams.mark, "mark", false, "Display dot on pod state change wait. Log if false")
	// This is a last resort parameter, as the child job should be cleanup up by its parent
	monitorCmd.PersistentFlags().DurationVar(&monitorParams.ttlAfterFinished, "ttlAfterFinished", time.Minute*30, "Wait before cleanup")

	_ = monitorCmd.MarkPersistentFlagRequired("image")
}

var monitorCmd = &cobra.Command{
	Use:   "monitor",
	Short: "Monitor OIDC plugin api server configuration",
	Run: func(cmd *cobra.Command, args []string) {

		// Set up logger
		logger, err := misc.NewLogger(&monitorParams.logConfig)
		if err != nil {
			fmt.Printf("Error creating logger: %v\n", err)
			os.Exit(1)
		}

		// Inject logger into context
		ctx := logr.NewContextWithSlogLogger(context.Background(), logger)

		config := &config.AcConfig{}
		if monitorParams.configFile != "" {
			err = misc.LoadYaml(monitorParams.configFile, config)
			if err != nil {
				_, _ = fmt.Fprintf(os.Stderr, "Unable to load configuration: %v\n", err)
				os.Exit(2)
			}
		}

		clientSet, err := k8sapi.GetClientSet("")
		if err != nil {
			logger.Error("Error creating Kubernetes client", "error", err)
			os.Exit(1)
		}

		logger.Info("OIDC plugin API Server configuration monitor", "version", global.Version, "build", global.BuildTs, "logLevel", monitorParams.logConfig.Level, "remove", monitorParams.remove)

		nodes, err := lookupApiServerNodes(ctx, clientSet, config)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "Error while listing APIs server nodes: %v\n", err)
			os.Exit(2)
		}
		sort.Strings(nodes) // To have a predictable order
		logger.Info("Lookup api server", "nodes", nodes)

		for idx, nodeName := range nodes {
			err := handleNodeJob(ctx, clientSet, config, idx, nodeName)
			if err != nil {
				_, _ = fmt.Fprintf(os.Stderr, "Error on job for '%s': %v\n", nodeName, err)
				os.Exit(2)
			}
		}
	},
}

func lookupApiServerNodes(ctx context.Context, clientSet *kubernetes.Clientset, config *config.AcConfig) ([]string, error) {
	pods, err := clientSet.CoreV1().Pods(config.ApiServerNamespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	result := make([]string, 0, 3)
	for _, pod := range pods.Items {
		if strings.HasPrefix(pod.Name, config.ApiServerPodName) {
			result = append(result, pod.Spec.NodeName)
		}
	}
	return result, nil
}

func handleNodeJob(ctx context.Context, clientSet *kubernetes.Clientset, config *config.AcConfig, idx int, nodeName string) error {
	logger := logr.FromContextAsSlogLogger(ctx)
	job, err := buildJob(config, idx, nodeName, logger)
	if err != nil {
		return err
	}
	logger.Info("handle node job", "jobName", job.Name)
	logger.Info("handle node job", "nodeName", job.Spec.Template.Spec.NodeName)
	logger.Info("handle node job", "image", job.Spec.Template.Spec.Containers[0].Image)
	_, err = clientSet.BatchV1().Jobs(config.KubauthKitNamespace).Create(ctx, job, metav1.CreateOptions{})
	if err != nil {
		return err
	}
	// And loop until job end
	logger.Info("Wait for child job to end", "nodeName", nodeName, "idx", idx)
	limit := time.Now().Add(config.TimeoutApiServerUp)
	for {
		time.Sleep(time.Second)
		job2, err := clientSet.BatchV1().Jobs(config.KubauthKitNamespace).Get(ctx, job.Name, metav1.GetOptions{})
		if err != nil {
			// Api server unreachable is a normal case on a 1 node control plane.
			if monitorParams.mark {
				fmt.Printf(":")
			}
			if time.Now().After(limit) {
				if monitorParams.mark {
					fmt.Printf("\n")
				}
				return fmt.Errorf("timeout on apiserver up expired. Last error: %v", err)
			}
		} else {
			ended, st := isJobFinished(job2)
			if ended {
				if st == batchv1.JobFailed {
					if monitorParams.mark {
						fmt.Printf("\n")
					}
					return fmt.Errorf("child job#%d failed (node:%s)", idx, nodeName)
				}
				if monitorParams.mark {
					fmt.Printf("\n")
				}
				logger.Info("child job OK", "nodeName", nodeName, "idx", idx)
				return nil
			} else {
				if monitorParams.mark {
					fmt.Printf(".")
				}
			}
		}
	}
}

/*
We consider a job "finished" if it has a "Complete" or "Failed" condition marked as true.
Status conditions allow us to add extensible status information to our objects that other
humans and controllers can examine to check things like completion and health.
*/
func isJobFinished(job *batchv1.Job) (bool, batchv1.JobConditionType) {
	for _, c := range job.Status.Conditions {
		if (c.Type == batchv1.JobComplete || c.Type == batchv1.JobFailed) && c.Status == corev1.ConditionTrue {
			return true, c.Type
		}
	}
	return false, ""
}

func buildOwnerReference(logger *slog.Logger) map[string]interface{} {
	myPodName := os.Getenv("MY_POD_NAME")
	myPodUid := os.Getenv("MY_POD_UID")
	if myPodName != "" && myPodUid != "" {
		oref := make(map[string]interface{})
		oref["name"] = myPodName
		oref["uid"] = myPodUid
		logger.Info("setting ownerReferences", "podName", myPodName, "uid", myPodUid)
		return oref
	} else {
		logger.Info("Unable to set ownerReferences. Missing MY_POD_NAME and/or MY_POD_UID environment variables")
		return nil
	}
}

// https://github.com/kubernetes/client-go/issues/193

func buildJob(config *config.AcConfig, idx int, nodeName string, logger *slog.Logger) (*batchv1.Job, error) {

	model := map[string]interface{}{
		"Config": config,
		"Values": map[string]interface{}{
			"idx":                     idx,
			"nodeName":                nodeName,
			"ownerReferences":         buildOwnerReference(logger),
			"ttlSecondsAfterFinished": monitorParams.ttlAfterFinished.Seconds(),
			"mark":                    monitorParams.mark,
			"remove":                  monitorParams.remove,
			"force":                   monitorParams.force,
			"log": map[string]interface{}{
				"level": monitorParams.logConfig.Level,
				"mode":  monitorParams.logConfig.Mode,
			},
		},
	}

	result, err := texttemplate.NewAndRenderToTextFromFile(monitorParams.jobTemplate, model)
	if err != nil {
		return nil, err
	}
	// fmt.Printf("\n==================== JOB:\n%s\n", result)
	job := &batchv1.Job{}
	err = yaml.NewYAMLToJSONDecoder(strings.NewReader(result)).Decode(job)
	if err != nil {
		return nil, err
	}
	return job, nil
}
