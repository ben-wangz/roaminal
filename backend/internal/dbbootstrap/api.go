package dbbootstrap

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

const serviceAccountPath = "/var/run/secrets/kubernetes.io/serviceaccount/"
const bootstrapTimeout = time.Minute

var (
	errNotFound = errors.New("bootstrap resource not found")
	errExists   = errors.New("bootstrap resource already exists")
)

type resource struct {
	APIVersion string            `json:"apiVersion,omitempty"`
	Kind       string            `json:"kind,omitempty"`
	Metadata   metadata          `json:"metadata"`
	Type       string            `json:"type,omitempty"`
	Data       map[string]string `json:"data,omitempty"`
	StringData map[string]string `json:"stringData,omitempty"`
}

type metadata struct {
	Name        string            `json:"name"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

type apiClient struct {
	base, token string
	http        *http.Client
}

func Run(ctx context.Context, c Config) error {
	if err := c.validate(); err != nil {
		return err
	}
	client, err := inClusterClient()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, bootstrapTimeout)
	defer cancel()
	return bootstrap(ctx, client, c)
}

func inClusterClient() (*apiClient, error) {
	host, port := os.Getenv("KUBERNETES_SERVICE_HOST"), os.Getenv("KUBERNETES_SERVICE_PORT")
	if net.ParseIP(host) == nil || port == "" {
		return nil, errors.New("Kubernetes service address unavailable")
	}
	ca, err := os.ReadFile(serviceAccountPath + "ca.crt")
	roots := x509.NewCertPool()
	if err != nil || !roots.AppendCertsFromPEM(ca) {
		return nil, errors.New("Kubernetes service CA unavailable")
	}
	tokenBytes, err := os.ReadFile(serviceAccountPath + "token")
	token := strings.TrimSpace(string(tokenBytes))
	if err != nil || token == "" {
		return nil, errors.New("Kubernetes service account token unavailable")
	}
	return &apiClient{base: "https://" + net.JoinHostPort(host, port), token: token, http: &http.Client{
		Transport:     &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Timeout:       5 * time.Second,
	}}, nil
}

func (a *apiClient) request(ctx context.Context, method, path string, input *resource) (resource, error) {
	var body io.Reader
	if input != nil {
		payload, err := json.Marshal(input)
		if err != nil {
			return resource{}, errors.New("encode bootstrap resource")
		}
		body = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, a.base+path, body)
	if err != nil {
		return resource{}, errors.New("prepare Kubernetes bootstrap request")
	}
	req.Header.Set("Authorization", "Bearer "+a.token)
	if input != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := a.http.Do(req)
	if err != nil {
		return resource{}, errors.New("Kubernetes bootstrap request failed")
	}
	defer res.Body.Close()
	switch res.StatusCode {
	case http.StatusNotFound:
		return resource{}, errNotFound
	case http.StatusConflict:
		return resource{}, errExists
	case http.StatusOK, http.StatusCreated:
		var result resource
		if err := json.NewDecoder(io.LimitReader(res.Body, 2<<20)).Decode(&result); err != nil {
			return resource{}, errors.New("decode Kubernetes bootstrap resource")
		}
		return result, nil
	default:
		// Kubernetes error bodies may echo input fields, including credentials.
		return resource{}, fmt.Errorf("Kubernetes bootstrap API returned HTTP %d", res.StatusCode)
	}
}
