// Package mimir provides client implementations for interacting with Grafana Mimir APIs.
package mimir

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/grafana/dskit/crypto/tls"
	"github.com/grafana/dskit/user"
)

const (
	rulerAPIPath  = "/prometheus/config/v1/rules"
	legacyAPIPath = "/api/v1/rules"
)

var (
	// ErrResourceNotFound indicates the requested resource was not found (404)
	ErrResourceNotFound = errors.New("requested resource not found")
	errConflict         = errors.New("conflict with current state of target resource")
	errTooManyRequests  = errors.New("too many requests")
)

// UserAgent returns build information in format suitable to be used in HTTP User-Agent header.
func UserAgent() string {
	return "openawareness.operator"
}

// Config is used to configure a MimirClient.
type Config struct {
	User            string `yaml:"user"`
	Key             string `yaml:"key"`
	Address         string `yaml:"address"`
	TLS             tls.ClientConfig
	UseLegacyRoutes bool              `yaml:"use_legacy_routes"`
	MimirHTTPPrefix string            `yaml:"mimir_http_prefix"`
	AuthToken       string            `yaml:"auth_token"`
	ExtraHeaders    map[string]string `yaml:"extra_headers"`
}

// Client is a client to the Mimir API.
type Client struct {
	user         string
	key          string
	id           string
	endpoint     *url.URL
	Client       http.Client
	apiPath      string
	authToken    string
	extraHeaders map[string]string
	log          logr.Logger
}

// New returns a new Client.
func New(ctx context.Context, cfg Config) (*Client, error) {
	logger := log.FromContext(ctx)
	endpoint, err := url.Parse(cfg.Address)
	if err != nil {
		return nil, err
	}

	logger.Info("New Mimir client created",
		"address", cfg.Address)

	client := http.Client{}

	// Setup TLS client
	tlsConfig, err := cfg.TLS.GetTLSConfig()
	if err != nil {
		logger.Error(err, "Mimir client initialization unsuccessful",
			"tls-ca", cfg.TLS.CAPath,
			"tls-cert", cfg.TLS.CertPath,
			"tls-key", cfg.TLS.KeyPath,
		)
		return nil, fmt.Errorf("mimir client initialization unsuccessful")
	}

	if tlsConfig != nil {
		transport := &http.Transport{
			Proxy:           http.ProxyFromEnvironment,
			TLSClientConfig: tlsConfig,
		}
		client = http.Client{Transport: transport}
	}

	path := rulerAPIPath
	if cfg.UseLegacyRoutes {
		var err error
		if path, err = url.JoinPath(cfg.MimirHTTPPrefix, legacyAPIPath); err != nil {
			return nil, err
		}
	}

	return &Client{
		user:         cfg.User,
		key:          cfg.Key,
		endpoint:     endpoint,
		Client:       client,
		apiPath:      path,
		authToken:    cfg.AuthToken,
		extraHeaders: cfg.ExtraHeaders,
		log:          logger,
	}, nil
}

// HealthCheck performs a lightweight health check by attempting to list rules
// for an empty namespace. This verifies connectivity, authentication, and basic API access.
func (r *Client) HealthCheck(ctx context.Context) error {
	r.log.V(1).Info("Performing health check")

	// Use a simple API call to verify connectivity
	// List rules for a system namespace that should always be accessible
	req := r.apiPath

	res, err := r.doRequest(ctx, req, "GET", nil, -1, "")
	if err != nil {
		r.log.Error(err, "Health check failed")
		return err
	}
	defer func() { _ = res.Body.Close() }()

	r.log.Info("Health check successful", "status", res.Status)
	return nil
}

// Query executes a PromQL query against the Mimir cluster.
func (r *Client) Query(ctx context.Context, query string, tenantID string) (*http.Response, error) {
	req := fmt.Sprintf("/prometheus/api/v1/query?query=%s&time=%d", url.QueryEscape(query), time.Now().Unix())

	res, err := r.doRequest(ctx, req, "GET", nil, -1, tenantID)
	if err != nil {
		return nil, err
	}

	return res, nil
}

func (r *Client) doRequest(
	ctx context.Context,
	path, method string,
	payload io.Reader,
	contentLength int64,
	tenantID string,
) (*http.Response, error) {
	req, err := buildRequest(ctx, path, method, *r.endpoint, payload, contentLength)
	if err != nil {
		return nil, err
	}

	switch {
	case (r.user != "" || r.key != "") && r.authToken != "":
		err := errors.New("at most one of basic auth or auth token should be configured")
		r.log.Error(err, "error during setting up request to mimir api",
			"url", req.URL.String(),
			"method", req.Method,
		)
		return nil, err

	case r.user != "":
		req.SetBasicAuth(r.user, r.key)

	case r.key != "":
		req.SetBasicAuth(r.id, r.key)

	case r.authToken != "":
		req.Header.Add("Authorization", "Bearer "+r.authToken)
	}

	for k, v := range r.extraHeaders {
		req.Header.Add(k, v)
	}

	// Use provided tenant ID if given, otherwise fall back to client's default tenant ID
	if tenantID != "" {
		req.Header.Add(user.OrgIDHeaderName, tenantID)
	} else {
		req.Header.Add(user.OrgIDHeaderName, r.id)
	}

	r.log.Info("sending request to Grafana Mimir API",
		"url", req.URL.String(),
		"method", req.Method)

	resp, err := r.Client.Do(req)
	if err != nil {
		r.log.Error(err, "error during request to Grafana Mimir API",
			"url", req.URL.String(),
			"method", req.Method,
		)
		return nil, err
	}

	if err := r.checkResponse(resp); err != nil {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("%w, %s request to %s failed", err, req.Method, req.URL.String())
	}

	return resp, nil
}

// checkResponse checks an API response for errors.
func (r *Client) checkResponse(resp *http.Response) error {
	r.log.Info("checking response", "status", resp.Status)

	if 200 <= resp.StatusCode && resp.StatusCode <= 299 {
		return nil
	}

	bodyHead, err := io.ReadAll(io.LimitReader(resp.Body, 1024))
	if err != nil {
		return fmt.Errorf("reading body: %w", err)
	}
	bodyStr := string(bodyHead)
	const msg = "response"
	if resp.StatusCode == http.StatusNotFound {
		r.log.Info(msg,
			"status", resp.Status,
			"body", bodyStr,
		)
		return ErrResourceNotFound
	}
	if resp.StatusCode == http.StatusConflict {
		r.log.Info(msg,
			"status", resp.Status,
			"body", bodyStr,
		)
		return errConflict
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		r.log.Info(msg,
			"status", resp.Status,
			"body", bodyStr,
		)
		return errTooManyRequests
	}

	r.log.Info(msg,
		"status", resp.Status,
		"body", bodyStr,
	)

	var errMsg string
	if bodyStr == "" {
		errMsg = fmt.Sprintf("server returned HTTP status: %s", resp.Status)
	} else {
		errMsg = fmt.Sprintf("server returned HTTP status: %s, body: %q", resp.Status, bodyStr)
	}

	return errors.New(errMsg)
}

func joinPath(baseURLPath, targetPath string) string {
	// trim exactly one slash at the end of the base URL, this expects target
	// path to always start with a slash
	return strings.TrimSuffix(baseURLPath, "/") + targetPath
}

func buildRequest(
	ctx context.Context,
	p, m string,
	endpoint url.URL,
	payload io.Reader,
	contentLength int64,
) (*http.Request, error) {
	// parse path parameter again (as it already contains escaped path information
	pURL, err := url.Parse(p)
	if err != nil {
		return nil, err
	}

	// if path or endpoint contains escaping that requires RawPath to be populated, also join rawPath
	if pURL.RawPath != "" || endpoint.RawPath != "" {
		endpoint.RawPath = joinPath(endpoint.EscapedPath(), pURL.EscapedPath())
	}
	endpoint.Path = joinPath(endpoint.Path, pURL.Path)
	endpoint.RawQuery = pURL.RawQuery
	r, err := http.NewRequestWithContext(ctx, m, endpoint.String(), payload)
	if err != nil {
		return nil, err
	}
	if contentLength >= 0 {
		r.ContentLength = contentLength
	}
	r.Header.Add("User-Agent", UserAgent())
	return r, nil
}
