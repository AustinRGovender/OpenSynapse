// Package kube is a minimal Kubernetes API client for the few calls cluster
// mode makes (creating k6-operator TestRuns, their ConfigMaps and Secrets,
// listing runner pods and following their logs). It authenticates with the
// pod's service account, so it only works inside a cluster.
package kube

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
	"net/url"
	"os"
	"strings"
	"time"
)

const serviceAccountDir = "/var/run/secrets/kubernetes.io/serviceaccount"

// Client talks to one cluster's API server, in one namespace.
type Client struct {
	base      string
	namespace string
	tokenPath string
	token     string // fixed token (tests); otherwise read from tokenPath per request
	http      *http.Client
	stream    *http.Client // no overall timeout, for log follows
}

// InCluster builds a client from the pod's service account.
func InCluster() (*Client, error) {
	host, port := os.Getenv("KUBERNETES_SERVICE_HOST"), os.Getenv("KUBERNETES_SERVICE_PORT")
	if host == "" || port == "" {
		return nil, errors.New("not running inside Kubernetes (KUBERNETES_SERVICE_HOST is not set)")
	}
	ca, err := os.ReadFile(serviceAccountDir + "/ca.crt")
	if err != nil {
		return nil, fmt.Errorf("read service account CA: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(ca) {
		return nil, errors.New("service account CA is not valid PEM")
	}
	ns, err := os.ReadFile(serviceAccountDir + "/namespace")
	if err != nil {
		return nil, fmt.Errorf("read service account namespace: %w", err)
	}
	transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}}
	return &Client{
		base:      "https://" + net.JoinHostPort(host, port),
		namespace: strings.TrimSpace(string(ns)),
		tokenPath: serviceAccountDir + "/token",
		http:      &http.Client{Transport: transport, Timeout: 30 * time.Second},
		stream:    &http.Client{Transport: transport},
	}, nil
}

// NewForTest points a client at a fake API server.
func NewForTest(base, namespace, token string, hc *http.Client) *Client {
	return &Client{base: strings.TrimRight(base, "/"), namespace: namespace, token: token, http: hc, stream: hc}
}

// Namespace is where the control plane runs and creates its resources.
func (c *Client) Namespace() string { return c.namespace }

// StatusError is a non-2xx answer from the API server.
type StatusError struct {
	Code    int
	Reason  string
	Message string
}

func (e *StatusError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("kubernetes API %d %s: %s", e.Code, e.Reason, e.Message)
	}
	return fmt.Sprintf("kubernetes API returned %d", e.Code)
}

// IsNotFound reports whether err is a 404 from the API server.
func IsNotFound(err error) bool {
	var se *StatusError
	return errors.As(err, &se) && se.Code == http.StatusNotFound
}

func (c *Client) bearer() (string, error) {
	if c.token != "" {
		return c.token, nil
	}
	// Projected tokens rotate; read the current one each time.
	b, err := os.ReadFile(c.tokenPath)
	if err != nil {
		return "", fmt.Errorf("read service account token: %w", err)
	}
	return strings.TrimSpace(string(b)), nil
}

func (c *Client) request(ctx context.Context, method, path string, body any) (*http.Request, error) {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rd)
	if err != nil {
		return nil, err
	}
	token, err := c.bearer()
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

func statusError(resp *http.Response) error {
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	var st struct {
		Reason  string `json:"reason"`
		Message string `json:"message"`
	}
	json.Unmarshal(b, &st)
	if st.Message == "" {
		st.Message = strings.TrimSpace(string(b))
	}
	return &StatusError{Code: resp.StatusCode, Reason: st.Reason, Message: st.Message}
}

// Do sends a JSON request and decodes a JSON answer into out (if non-nil).
func (c *Client) Do(ctx context.Context, method, path string, body, out any) error {
	req, err := c.request(ctx, method, path, body)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("kubernetes API: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return statusError(resp)
	}
	if out == nil {
		io.Copy(io.Discard, resp.Body)
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// Path builds a namespaced resource path, e.g.
// Path("", "v1", "configmaps", "x") → /api/v1/namespaces/<ns>/configmaps/x and
// Path("k6.io", "v1alpha1", "testruns", "") for a custom resource collection.
func (c *Client) Path(group, version, resource, name string) string {
	prefix := "/api/" + version
	if group != "" {
		prefix = "/apis/" + group + "/" + version
	}
	p := prefix + "/namespaces/" + url.PathEscape(c.namespace) + "/" + resource
	if name != "" {
		p += "/" + url.PathEscape(name)
	}
	return p
}

// Delete removes a resource and lets Kubernetes garbage-collect its dependents.
// A missing resource is not an error.
func (c *Client) Delete(ctx context.Context, path string) error {
	err := c.Do(ctx, http.MethodDelete, path, map[string]any{
		"apiVersion": "v1", "kind": "DeleteOptions", "propagationPolicy": "Background",
	}, nil)
	if IsNotFound(err) {
		return nil
	}
	return err
}

// FollowLogs streams a container's log from the start (or from since, if
// set), each line prefixed with its RFC 3339 timestamp.
func (c *Client) FollowLogs(ctx context.Context, pod, container string, since time.Time) (io.ReadCloser, error) {
	q := url.Values{"follow": {"true"}, "timestamps": {"true"}}
	if container != "" {
		q.Set("container", container)
	}
	if !since.IsZero() {
		q.Set("sinceTime", since.UTC().Format(time.RFC3339))
	}
	req, err := c.request(ctx, http.MethodGet, c.Path("", "v1", "pods", pod)+"/log?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.stream.Do(req)
	if err != nil {
		return nil, fmt.Errorf("kubernetes API: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return nil, statusError(resp)
	}
	return resp.Body, nil
}

// TailLogs returns the last lines of a pod's log.
func (c *Client) TailLogs(ctx context.Context, pod string, lines int) (string, error) {
	q := url.Values{"tailLines": {fmt.Sprint(lines)}}
	req, err := c.request(ctx, http.MethodGet, c.Path("", "v1", "pods", pod)+"/log?"+q.Encode(), nil)
	if err != nil {
		return "", err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("kubernetes API: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", statusError(resp)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	return string(b), err
}
