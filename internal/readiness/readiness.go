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

package readiness

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"kubauth-apiserver/internal/config"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
)

type Probe interface {
	IsReady() error
	WaitForDown(timeout time.Duration, mark bool) error
	WaitForUp(timeout time.Duration, mark bool) error
}

type probe struct {
	url string
	//*v1.HTTPGetAction
	httpClient *http.Client
	logger     *slog.Logger
	pod        string // For logs
	//token      []byte
}

var _ Probe = &probe{}

// resolvePort resolves a port from intstr.IntOrString to a string representation.
// If the port is numeric, it returns the string representation.
// If the port is a named port, it looks up the actual port number from the container's ports.
func resolvePort(port intstr.IntOrString, container v1.Container) (string, error) {
	if port.Type == intstr.Int {
		return strconv.Itoa(port.IntValue()), nil
	}

	// Port is a string (named port), need to look it up in container ports
	namedPort := port.String()
	for _, containerPort := range container.Ports {
		if containerPort.Name == namedPort {
			return strconv.Itoa(int(containerPort.ContainerPort)), nil
		}
	}

	return "", fmt.Errorf("named port '%s' not found in container ports", namedPort)
}

func GetProbe(ctx context.Context, clientSet *kubernetes.Clientset, config *config.AcConfig, nodeName string) (Probe, error) {
	logger := logr.FromContextAsSlogLogger(ctx)
	// First, list pod
	pods, err := clientSet.CoreV1().Pods(config.ApiServerNamespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	foundApiSrv := 0
	for _, pod := range pods.Items {
		if strings.HasPrefix(pod.Name, config.ApiServerPodName) {
			foundApiSrv++
			if pod.Spec.NodeName == nodeName {
				logger = logger.With("pod", pod.Name)
				logger.Debug("Find pod")
				container := pod.Spec.Containers[0]
				ep := container.ReadinessProbe.HTTPGet

				// Resolve the port (handles both numeric and named ports)
				resolvedPort, err := resolvePort(ep.Port, container)
				if err != nil {
					return nil, fmt.Errorf("failed to resolve port: %w", err)
				}

				u := fmt.Sprintf("%s://%s:%s", ep.Scheme, ep.Host, resolvedPort)
				u, err = url.JoinPath(u, ep.Path)
				if err != nil {
					return nil, err
				}
				probe := &probe{
					url:    u,
					logger: logger,
					pod:    pod.Name,
				}
				probe.httpClient, err = buildHttpClient(pod.Spec.Containers[0].ReadinessProbe.HTTPGet.Scheme, config.KubernetesCAPath)
				if err != nil {
					return nil, err
				}
				// To validate the url
				_, err = http.NewRequest("GET", probe.url, nil)
				if err != nil {
					return nil, err
				}
				//probe.token, err = os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/token")
				//if err != nil {
				//	return nil, err
				//}
				logger.Info("Setup Readiness", "url", probe.url)
				return probe, nil
			}
		}
	}
	if foundApiSrv == 0 {
		return nil, fmt.Errorf("no pod namad %s-... found in namespace '%s'", config.ApiServerPodName, config.ApiServerNamespace)
	} else {
		return nil, fmt.Errorf("no pod namad %s-... found for nodeName '%s'", config.ApiServerPodName, nodeName)
	}
}

func (p *probe) IsReady() error {
	req, err := http.NewRequest("GET", p.url, nil)
	if err != nil {
		return err // Should not occurs, as http.NewRequest() has been tested in GetProbe()
	}
	//req.Header.Set("Authorization", "Bearer "+string(p.token))
	resp, err := p.httpClient.Do(req)
	if err != nil {
		p.logger.Debug("httpClient.Do() failed", "error", err)
		return err
	}
	ba, err := io.ReadAll(resp.Body)
	if err != nil {
		p.logger.Debug("io.ReadAll() failed", "error", err)
		return err
	}
	p.logger.Debug("IsReady", "status", resp.Status, "statusCode", resp.StatusCode, "body", string(ba))
	if resp.StatusCode != 200 {
		return fmt.Errorf("statudCode:%d", resp.StatusCode)
	}
	return nil
}

func (p *probe) WaitForDown(timeout time.Duration, mark bool) error {
	limit := time.Now().Add(timeout)
	if mark {
		fmt.Printf("Wait for %s down:", p.pod)
	} else {
		p.logger.Info("Wait for pod down")
	}
	for {
		if mark {
			fmt.Printf(".")
		}
		if p.IsReady() != nil {
			if mark {
				fmt.Printf("DOWN\n")
			} else {
				p.logger.Info("pod DOWN")
			}
			return nil
		}
		//fmt.Printf("timeout: %s\n  now: %s\nlimit: %s\n", timeout, time.Now(), limit)
		if time.Now().After(limit) {
			if mark {
				fmt.Printf("TIMED OUT!\n")
			}
			return fmt.Errorf("time out expired on waitForDown(%s)", p.pod)
		}
		time.Sleep(time.Millisecond * 1000)
	}
}

func (p *probe) WaitForUp(timeout time.Duration, mark bool) error {
	limit := time.Now().Add(timeout)
	if mark {
		fmt.Printf("Wait for %s up:", p.pod)
	} else {
		p.logger.Info("Wait for pod up")
	}
	for {
		if mark {
			fmt.Printf(".")
		}
		if p.IsReady() == nil {
			if mark {
				fmt.Printf("UP\n")
			} else {
				p.logger.Info("pod UP")
			}
			return nil
		}
		//fmt.Printf("timeout: %s\n  now: %s\nlimit: %s\n", timeout, time.Now(), limit)
		if time.Now().After(limit) {
			if mark {
				fmt.Printf("TIMED OUT!\n")
			}
			return fmt.Errorf("time out expired on waitforUp(%s)", p.pod)
		}
		time.Sleep(time.Millisecond * 1000)
	}
}

func buildHttpClient(scheme v1.URIScheme, rootCAPath string) (*http.Client, error) {
	var tlsConfig *tls.Config = nil
	if strings.ToLower(string(scheme)) == "https" {
		pool, err := x509.SystemCertPool()
		if err != nil {
			return nil, err
		}
		tlsConfig = &tls.Config{RootCAs: pool, InsecureSkipVerify: false}
		err = appendCaFromFile(tlsConfig.RootCAs, rootCAPath)
		if err != nil {
			return nil, err
		}
	}
	httpclient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
			Proxy:           http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}
	return httpclient, nil
}

func appendCaFromFile(pool *x509.CertPool, caPath string) error {
	rootCaBytes, err := os.ReadFile(caPath)
	if err != nil {
		return fmt.Errorf("failed to read CA file '%s': %w", caPath, err)
	}
	if !pool.AppendCertsFromPEM(rootCaBytes) {
		return fmt.Errorf("invalid root CA certificate in file %s", caPath)
	}
	return nil
}
