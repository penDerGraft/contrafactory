// Package client provides a Go client for the Contrafactory API.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Client is a Contrafactory API client
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// Option configures a Client
type Option func(*Client)

// WithHTTPClient sets a custom HTTP client
func WithHTTPClient(c *http.Client) Option {
	return func(client *Client) {
		client.httpClient = c
	}
}

// New creates a new Contrafactory client
func New(baseURL, apiKey string, opts ...Option) *Client {
	c := &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// Package represents a package in the registry
type Package struct {
	Name            string   `json:"name"`
	Version         string   `json:"version,omitempty"`
	Chain           string   `json:"chain,omitempty"`
	Builder         string   `json:"builder,omitempty"`
	CompilerVersion string   `json:"compilerVersion,omitempty"`
	Contracts       []string `json:"contracts,omitempty"`
	CreatedAt       string   `json:"createdAt,omitempty"`
	Versions        []string `json:"versions,omitempty"`
}

// Contract represents a contract in a package
type Contract struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Chain       string `json:"chain"`
	SourcePath  string `json:"sourcePath"`
	License     string `json:"license,omitempty"`
	PrimaryHash string `json:"primaryHash"`
	CreatedAt   string `json:"createdAt"`
}

// Deployment represents a recorded deployment
type Deployment struct {
	ID              string `json:"id"`
	PackageID       string `json:"packageId"`
	ContractName    string `json:"contractName"`
	Chain           string `json:"chain"`
	ChainID         string `json:"chainId"`
	Address         string `json:"address"`
	DeployerAddress string `json:"deployerAddress,omitempty"`
	TxHash          string `json:"txHash,omitempty"`
	BlockNumber     int64  `json:"blockNumber,omitempty"`
	Verified        bool   `json:"verified"`
	CreatedAt       string `json:"createdAt"`
}

// PublishRequest is the request for publishing a package
type PublishRequest struct {
	Chain     string     `json:"chain"`
	Builder   string     `json:"builder,omitempty"`
	Artifacts []Artifact `json:"artifacts"`
}

// Artifact represents a contract artifact for publishing
type Artifact struct {
	Name              string          `json:"name"`
	SourcePath        string          `json:"sourcePath"`
	License           string          `json:"license,omitempty"`
	ABI               json.RawMessage `json:"abi"`
	Bytecode          string          `json:"bytecode"`
	DeployedBytecode  string          `json:"deployedBytecode"`
	StandardJSONInput json.RawMessage `json:"standardJsonInput,omitempty"`
}

// DeploymentRequest is the request for recording a deployment
type DeploymentRequest struct {
	Package         string            `json:"package"`
	Version         string            `json:"version"`
	Contract        string            `json:"contract"`
	ChainID         int               `json:"chainId"`
	Address         string            `json:"address"`
	TxHash          string            `json:"txHash,omitempty"`
	DeployerAddress string            `json:"deployerAddress,omitempty"`
	BlockNumber     int64             `json:"blockNumber,omitempty"`
	ConstructorArgs string            `json:"constructorArgs,omitempty"`
	Libraries       map[string]string `json:"libraries,omitempty"`
}

// ListPackagesResponse is the response for listing packages
type ListPackagesResponse struct {
	Data       []Package  `json:"data"`
	Pagination Pagination `json:"pagination"`
}

// Pagination contains pagination info
type Pagination struct {
	Limit      int    `json:"limit"`
	HasMore    bool   `json:"hasMore"`
	NextCursor string `json:"nextCursor,omitempty"`
}

// APIError represents an API error response
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// ListPackages lists packages in the registry
func (c *Client) ListPackages(ctx context.Context) (*ListPackagesResponse, error) {
	var resp ListPackagesResponse
	if err := c.get(ctx, "/api/v1/packages", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetPackage gets a package by name
func (c *Client) GetPackage(ctx context.Context, name string) (*Package, error) {
	var resp Package
	if err := c.get(ctx, "/api/v1/packages/"+url.PathEscape(name), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetPackageVersion gets a specific package version
func (c *Client) GetPackageVersion(ctx context.Context, name, version string) (*Package, error) {
	var resp Package
	path := fmt.Sprintf("/api/v1/packages/%s/%s", url.PathEscape(name), url.PathEscape(version))
	if err := c.get(ctx, path, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Publish publishes a new package version
func (c *Client) Publish(ctx context.Context, name, version string, req PublishRequest) error {
	path := fmt.Sprintf("/api/v1/packages/%s/%s", url.PathEscape(name), url.PathEscape(version))
	return c.post(ctx, path, req, nil)
}

// GetABI gets the ABI for a contract
func (c *Client) GetABI(ctx context.Context, name, version, contract string) (json.RawMessage, error) {
	path := fmt.Sprintf("/api/v1/packages/%s/%s/contracts/%s/abi",
		url.PathEscape(name), url.PathEscape(version), url.PathEscape(contract))
	return c.getRaw(ctx, path)
}

// GetBytecode gets the bytecode for a contract
func (c *Client) GetBytecode(ctx context.Context, name, version, contract string) ([]byte, error) {
	path := fmt.Sprintf("/api/v1/packages/%s/%s/contracts/%s/bytecode",
		url.PathEscape(name), url.PathEscape(version), url.PathEscape(contract))
	return c.getRaw(ctx, path)
}

// GetDeployedBytecode gets the deployed bytecode for a contract
func (c *Client) GetDeployedBytecode(ctx context.Context, name, version, contract string) ([]byte, error) {
	path := fmt.Sprintf("/api/v1/packages/%s/%s/contracts/%s/deployed-bytecode",
		url.PathEscape(name), url.PathEscape(version), url.PathEscape(contract))
	return c.getRaw(ctx, path)
}

// GetStandardJSONInput gets the standard JSON input for a contract
func (c *Client) GetStandardJSONInput(ctx context.Context, name, version, contract string) ([]byte, error) {
	path := fmt.Sprintf("/api/v1/packages/%s/%s/contracts/%s/standard-json-input",
		url.PathEscape(name), url.PathEscape(version), url.PathEscape(contract))
	return c.getRaw(ctx, path)
}

// RecordDeployment records a deployment
func (c *Client) RecordDeployment(ctx context.Context, req DeploymentRequest) error {
	return c.post(ctx, "/api/v1/deployments", req, nil)
}

// GetDeployment gets a deployment by chain ID and address
func (c *Client) GetDeployment(ctx context.Context, chainID, address string) (*Deployment, error) {
	var resp Deployment
	path := fmt.Sprintf("/api/v1/deployments/%s/%s", url.PathEscape(chainID), url.PathEscape(address))
	if err := c.get(ctx, path, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) get(ctx context.Context, path string, result any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}

	return c.do(req, result)
}

func (c *Client) getRaw(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}

	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, c.parseError(resp)
	}

	return io.ReadAll(resp.Body)
}

func (c *Client) post(ctx context.Context, path string, body, result any) error {
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			return err
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	return c.do(req, result)
}

func (c *Client) do(req *http.Request, result any) error {
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return c.parseError(resp)
	}

	if result != nil {
		return json.NewDecoder(resp.Body).Decode(result)
	}

	return nil
}

func (c *Client) setHeaders(req *http.Request) {
	if c.apiKey != "" {
		req.Header.Set("X-API-Key", c.apiKey)
	}
	req.Header.Set("Accept", "application/json")
}

func (c *Client) parseError(resp *http.Response) error {
	var errResp struct {
		Error APIError `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}
	return &errResp.Error
}
