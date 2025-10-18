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

package config

import "time"

type AcConfig struct {
	ApiServerNamespace        string   `yaml:"apiServerNamespace"`
	ApiServerPodName          string   `yaml:"apiServerPodName"`
	PatcherImage              string   `yaml:"patcherImage"` // image for the patcher
	PatcherImagePullPolicy    string   `yaml:"patcherImagePullPolicy"`
	PatcherImagePullSecrets   []string `yaml:"patcherImagePullSecrets"`
	PatcherServiceAccountName string   `yaml:"patcherServiceAccountName"`
	PatcherJobName            string   `yaml:"patcherJobName"`
	UnPatcherJobName          string   `yaml:"unPatcherJobName"`
	ConfigMapName             string   `yaml:"configMapName"` // This config map
	// ----------------------------- Used by patcher
	// The 5 following values are interpreted inside the container, so depends on the 'hostPath' configuration
	ApiServerManifestPath string        `yaml:"apiServerManifestPath"`
	KubernetesCAPath      string        `yaml:"kubernetesCAPath"` // Used by http client to call api server pod's probes
	KubauthKitFolder      string        `yaml:"kubauthKitFolder"` // i/e: /etc/kubernetes/kubauth-kit
	BackupFolder          string        `yaml:"backupFolder"`
	TmpFolder             string        `yaml:"tmpFolder"`
	KubauthKitNamespace   string        `yaml:"kubauthKitNamespace"`
	TimeoutApiServerDown  time.Duration `yaml:"timeoutApiServerDown"` // Timeout on apiserver stop after patch
	TimeoutApiServerUp    time.Duration `yaml:"timeoutApiServerUp"`   // Timeout on apiserver restart after patch
	//// This is where to look up the CA used to validate the issuer certificate
	//IssuerCertificateAuthority struct {
	//	Secret struct {
	//		Namespace string `yaml:"namespace"`
	//		Name      string `yaml:"name"`
	//	}
	//	KeyInData string `yaml:"keyInData"`
	//} `yaml:"issuerCertificateAuthority"`
	Oidc struct {
		IssuerURL          string            `yaml:"issuerURL"`
		IssuerCaData       string            `yaml:"issuerCaData" json:"issuerCaData"`
		IssuerCaSecretName string            `yaml:"issuerCaSecretName" json:"issuerCaSecretName"` // Will be used to fulfill IssuerCaData if not set
		IssuerCaName       string            `yaml:"issuerCaName" json:"issuerCaName"`             // Will be used to fulfill IssuerCaData if not set
		ClientId           string            `yaml:"clientId"`
		UsernameClaim      string            `yaml:"usernameClaim"`
		UsernamePrefix     string            `yaml:"usernamePrefix"`
		GroupsClaim        string            `yaml:"groupsClaim"`
		GroupsPrefix       string            `yaml:"groupsPrefix"`
		RequiredClaims     map[string]string `yaml:"requiredClaims"`
	} `yaml:"oidc"`
}
