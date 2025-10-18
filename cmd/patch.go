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
	"kubauth-apiserver/internal/filepatcher"
	"kubauth-apiserver/internal/global"
	"kubauth-apiserver/internal/k8sapi"
	"kubauth-apiserver/internal/misc"
	"kubauth-apiserver/internal/readiness"
	"kubauth-apiserver/internal/texttemplate"
	"os"
	"path"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/spf13/cobra"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes"
)

var patchParams struct {
	logConfig       misc.LogConfig
	kubeconfig      string
	configFile      string
	nodeName        string
	remove          bool
	mark            bool
	force           bool
	patcherTemplate string
	namespace       string
}

func init() {
	patchCmd.PersistentFlags().StringVar(&patchParams.logConfig.Level, "logLevel", "INFO", "Log level")
	patchCmd.PersistentFlags().StringVar(&patchParams.logConfig.Mode, "logMode", "text", "Log mode: 'text' or 'json'")
	patchCmd.PersistentFlags().StringVar(&patchParams.kubeconfig, "kubeconfig", "", "kubeconfig file path. Override default configuration.")
	patchCmd.PersistentFlags().StringVar(&patchParams.configFile, "configFile", "/config.yaml", "Configuration file")
	patchCmd.PersistentFlags().BoolVar(&patchParams.remove, "remove", false, "Remove webhook configuration")
	patchCmd.PersistentFlags().BoolVar(&patchParams.force, "force", false, "Perform even if api server is down")
	patchCmd.PersistentFlags().StringVar(&patchParams.nodeName, "nodeName", "", "Node Name")
	patchCmd.PersistentFlags().BoolVar(&patchParams.mark, "mark", false, "Display dot on pod state change wait. Log if false")
	patchCmd.PersistentFlags().StringVar(&patchParams.patcherTemplate, "patcherTemplate", "/templates/patcher.tmpl", "patcher file template")
	patchCmd.PersistentFlags().StringVar(&patchParams.namespace, "namespace", "", "kubauth-apiserver namespace. For out of kubernetes launch")

	_ = patchCmd.MarkPersistentFlagRequired("nodeName")
}

var patchCmd = &cobra.Command{
	Use:   "patch",
	Short: "Patch an api server configuration",
	Run: func(cmd *cobra.Command, args []string) {

		// Set up logger
		logger, err := misc.NewLogger(&patchParams.logConfig)
		if err != nil {
			fmt.Printf("Error creating logger: %v\n", err)
			os.Exit(1)
		}

		err = func() error {

			// Inject logger into context
			ctx := logr.NewContextWithSlogLogger(context.Background(), logger)

			config := &config.AcConfig{}
			if patchParams.configFile != "" {
				err = misc.LoadYaml(patchParams.configFile, config)
				if err != nil {
					return fmt.Errorf("error loading config file: %w", err)
				}
			}

			namespace := patchParams.namespace
			if namespace == "" {
				namespace, err = getCurrentNamespace()
				if err != nil {
					return fmt.Errorf("error getting current namespace: %w", err)
				}
			}

			clientSet, err := k8sapi.GetClientSet("")
			if err != nil {
				return fmt.Errorf("error getting k8s clientSet: %w", err)
			}
			logger.Info("Api server node OIDC configurator", "version", global.Version, "build", global.BuildTs, "logLevel", patchParams.logConfig.Level, "nodeName", patchParams.nodeName, "remove", patchParams.remove)

			// First, ensure the api server is ready
			probe, err := readiness.GetProbe(ctx, clientSet, config, patchParams.nodeName)
			if err != nil {
				return fmt.Errorf("error getting readiness probe: %w", err)
			}
			err = probe.IsReady()
			if err != nil {
				if !patchParams.force {
					return fmt.Errorf("API server pod is not ready. Will not patch (err:%v)", err)
				} else {
					logger.Info("API server pod is NOT ready. Perform operation anyway")
				}
			}
			// Create kubauthKit folder
			if err = makeDirectoryIfNotExists(config.KubauthKitFolder); err != nil {
				return fmt.Errorf("error making directory for kubauthkit folder: %w", err)
			}
			if err = makeDirectoryIfNotExists(config.BackupFolder); err != nil {
				return fmt.Errorf("error making directory for backup folder: %w", err)
			}
			if err = makeDirectoryIfNotExists(config.TmpFolder); err != nil {
				return fmt.Errorf("error making directory for tmp folder: %w", err)
			}

			// Patch
			if patchParams.remove {
				err = unConfigure(config)
				if err != nil {
					return fmt.Errorf("error on unconfigure api server: %w", err)
				}
			} else {
				err = configure(ctx, clientSet, config, namespace)
				if err != nil {
					return fmt.Errorf("error on configure api server: %w", err)
				}
			}

			// And wait for a restart cycle
			// There will be no restart if the file has not been modified. (redo of install or remove)
			// So, this is not an error
			_ = probe.WaitForDown(config.TimeoutApiServerDown, patchParams.mark)

			err = probe.WaitForUp(config.TimeoutApiServerUp, patchParams.mark)
			if err != nil {
				return fmt.Errorf("error waiting for api server to become ready: %w", err)
			}
			return nil
		}()
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}

		fmt.Printf("\nSUCCESS!!\n")
	},
}

func configure(ctx context.Context, clientSet *kubernetes.Clientset, config *config.AcConfig, namespace string) error {
	// And now the issuer CA certificate
	issuerCaData := []byte(config.Oidc.IssuerCaData)
	if issuerCaData == nil || len(issuerCaData) == 0 {
		secret, err := clientSet.CoreV1().Secrets(namespace).Get(ctx, config.Oidc.IssuerCaSecretName, v1.GetOptions{})
		if err != nil {
			return err
		}
		var ok bool
		issuerCaData, ok = secret.Data[config.Oidc.IssuerCaName]
		if !ok {
			return fmt.Errorf("unable to find data[%s] in secret %s:%s", config.Oidc.IssuerCaName, namespace, config.Oidc.IssuerCaSecretName)
		}
	}
	issuerCaFile := path.Join(config.KubauthKitFolder, config.Oidc.IssuerCaName)
	if err := os.WriteFile(issuerCaFile, issuerCaData, 0600); err != nil {
		return err
	}
	if err := patchApiServerManifest(config, issuerCaFile, false); err != nil {
		return err
	}
	return nil
}

func makeDirectoryIfNotExists(path string) error {
	_, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return os.Mkdir(path, os.ModeDir|0755)
		} else {
			return err
		}
	}
	return nil
}

func unConfigure(config *config.AcConfig) error {
	if err := makeDirectoryIfNotExists(config.BackupFolder); err != nil {
		return err
	}
	err := patchApiServerManifest(config, "", true)
	if err != nil {
		return err
	}
	return os.RemoveAll(config.KubauthKitFolder)
}

func patchApiServerManifest(config *config.AcConfig, issuerCaFile string, remove bool) error {

	model := map[string]interface{}{
		"Config": config,
		"Values": map[string]interface{}{
			"remove":       remove,
			"nowRFC3339":   time.Now().Format(time.RFC3339),
			"issuerCaFile": issuerCaFile,
		},
	}
	patchOp, err := texttemplate.NewAndRenderToTextFromFile(patchParams.patcherTemplate, model)
	if err != nil {
		// fmt.Printf("============================= Error creating patch template: %v\n", err)
		return err
	}

	// fmt.Printf("=================== PATCH\n%s\n", patchOp)
	patchOperation := &filepatcher.PatchOperation{}
	err = yaml.UnmarshalStrict([]byte(patchOp), patchOperation)
	if err != nil {
		return err
	}

	err = patchOperation.Run()
	if err != nil {
		return err
	}
	return nil
}

// getCurrentNamespace returns the namespace of the current pod
func getCurrentNamespace() (string, error) {
	namespaceFile := "/var/run/secrets/kubernetes.io/serviceaccount/namespace"
	data, err := os.ReadFile(namespaceFile)
	if err != nil {
		return "", fmt.Errorf("failed to read namespace file: %w. If running out or Kubernetes context, set namespace explicitly (--namespace)", err)
	}
	return strings.TrimSpace(string(data)), nil
}
