package netbox

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	baseURL     *url.URL
	token       string
	httpClient  *http.Client
	debugOutput io.Writer
}

func NewClient(baseURL, token string, httpClient *http.Client) (*Client, error) {
	parsedURL, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil {
		return nil, fmt.Errorf("parse NetBox URL: %w", err)
	}
	if parsedURL.Scheme == "" || parsedURL.Host == "" {
		return nil, fmt.Errorf("NetBox URL must be absolute: %q", baseURL)
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	return &Client{baseURL: parsedURL, token: token, httpClient: httpClient}, nil
}

func (client *Client) SetDebugOutput(output io.Writer) {
	client.debugOutput = output
}

func (client *Client) Validate(ctx context.Context) error {
	request, err := client.newRequest(ctx, http.MethodGet, "/api/status/")
	if err != nil {
		return err
	}
	statusCode, status, body, err := client.do(request)
	if err != nil {
		return fmt.Errorf("contact NetBox: %w", err)
	}
	if statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden {
		return fmt.Errorf("NetBox rejected the configured token (%s): %s", status, body)
	}
	if statusCode < 200 || statusCode >= 300 {
		return fmt.Errorf("validate NetBox token: unexpected status %s: %s", status, body)
	}
	return nil
}

func (client *Client) Services(ctx context.Context) ([]Service, error) {
	next := "/api/ipam/services/?limit=100"
	var services []Service
	parentMetadata := make(map[string]targetMetadata)
	for next != "" {
		request, err := client.newRequest(ctx, http.MethodGet, next)
		if err != nil {
			return nil, err
		}
		statusCode, status, body, err := client.do(request)
		if err != nil {
			return nil, fmt.Errorf("query NetBox services: %w", err)
		}
		if statusCode < 200 || statusCode >= 300 {
			return nil, fmt.Errorf("query NetBox services: unexpected status %s: %s", status, body)
		}

		var page servicePage
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, fmt.Errorf("decode NetBox services: %w", err)
		}
		for _, record := range page.Results {
			service, err := client.serviceFromRecord(ctx, record, parentMetadata)
			if err != nil {
				return nil, err
			}
			services = append(services, service)
		}
		next = page.Next
	}
	return services, nil
}

func (client *Client) serviceFromRecord(ctx context.Context, record serviceRecord, parentMetadata map[string]targetMetadata) (Service, error) {
	service := record.toService()
	parentURL := record.parentURL()
	if parentURL == "" {
		return service, nil
	}

	metadata, ok := parentMetadata[parentURL]
	if !ok {
		var err error
		metadata, err = client.targetMetadata(ctx, parentURL)
		if err != nil {
			return Service{}, fmt.Errorf("query service parent %q: %w", service.TargetName(), err)
		}
		parentMetadata[parentURL] = metadata
	}
	service.Site = metadata.Site.text()
	service.Role = metadata.Role.text()
	service.Tenant = metadata.Tenant.text()
	service.Platform = metadata.Platform.text()
	service.Status = metadata.Status.text()
	return service, nil
}

func (client *Client) targetMetadata(ctx context.Context, path string) (targetMetadata, error) {
	request, err := client.newRequest(ctx, http.MethodGet, path)
	if err != nil {
		return targetMetadata{}, err
	}
	statusCode, status, body, err := client.do(request)
	if err != nil {
		return targetMetadata{}, err
	}
	if statusCode < 200 || statusCode >= 300 {
		return targetMetadata{}, fmt.Errorf("unexpected status %s: %s", status, body)
	}
	var metadata targetMetadata
	if err := json.Unmarshal(body, &metadata); err != nil {
		return targetMetadata{}, fmt.Errorf("decode response: %w", err)
	}
	return metadata, nil
}

func (client *Client) newRequest(ctx context.Context, method, path string) (*http.Request, error) {
	relative, err := url.Parse(path)
	if err != nil {
		return nil, fmt.Errorf("parse NetBox path: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, method, client.baseURL.ResolveReference(relative).String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create NetBox request: %w", err)
	}
	if strings.HasPrefix(client.token, "nbt_") {
		request.Header.Set("Authorization", "Bearer "+client.token)
	} else {
		request.Header.Set("Authorization", "Token "+client.token)
	}
	request.Header.Set("Accept", "application/json")
	return request, nil
}

func (client *Client) do(request *http.Request) (int, string, []byte, error) {
	client.trace("--> %s %s\n", request.Method, request.URL)
	response, err := client.httpClient.Do(request)
	if err != nil {
		client.trace("<-- request error: %v\n", err)
		return 0, "", nil, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return 0, "", nil, fmt.Errorf("read response body: %w", err)
	}
	client.trace("<-- %s\n%s\n", response.Status, body)
	return response.StatusCode, response.Status, body, nil
}

func (client *Client) trace(format string, args ...any) {
	if client.debugOutput != nil {
		fmt.Fprintf(client.debugOutput, format, args...)
	}
}

type servicePage struct {
	Next    string          `json:"next"`
	Results []serviceRecord `json:"results"`
}

type serviceRecord struct {
	Name           string         `json:"name"`
	Protocol       protocolChoice `json:"protocol"`
	Ports          []int          `json:"ports"`
	IPAddresses    []ipAddress    `json:"ipaddresses"`
	Parent         parentObject   `json:"parent"`
	Device         namedObject    `json:"device"`
	VirtualMachine namedObject    `json:"virtual_machine"`
	Tags           []namedObject  `json:"tags"`
	CustomFields   map[string]any `json:"custom_fields"`
}

type protocolChoice string

func (protocol *protocolChoice) UnmarshalJSON(data []byte) error {
	var plain string
	if err := json.Unmarshal(data, &plain); err == nil {
		*protocol = protocolChoice(plain)
		return nil
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return fmt.Errorf("decode protocol choice: %w", err)
	}
	for _, fieldName := range []string{"value", "name", "slug", "display", "label"} {
		var value string
		if err := json.Unmarshal(fields[fieldName], &value); err == nil && value != "" {
			*protocol = protocolChoice(value)
			return nil
		}
	}
	return fmt.Errorf("decode protocol choice: no string identifier")
}

type namedObject struct {
	Name        string `json:"name"`
	Display     string `json:"display"`
	DisplayName string `json:"display_name"`
	Slug        string `json:"slug"`
	URL         string `json:"url"`
}

func (object namedObject) text() string {
	for _, value := range []string{object.Name, object.Display, object.DisplayName, object.Slug} {
		if value != "" {
			return value
		}
	}
	return ""
}

type parentObject struct {
	Name       string `json:"name"`
	Display    string `json:"display"`
	ObjectType string `json:"object_type"`
	URL        string `json:"url"`
}

type targetMetadata struct {
	Site     namedObject  `json:"site"`
	Role     namedObject  `json:"role"`
	Tenant   namedObject  `json:"tenant"`
	Platform namedObject  `json:"platform"`
	Status   statusChoice `json:"status"`
}

type statusChoice string

func (status *statusChoice) UnmarshalJSON(data []byte) error {
	var plain string
	if err := json.Unmarshal(data, &plain); err == nil {
		*status = statusChoice(plain)
		return nil
	}
	var choice struct {
		Value string `json:"value"`
		Label string `json:"label"`
	}
	if err := json.Unmarshal(data, &choice); err != nil {
		return fmt.Errorf("decode status: %w", err)
	}
	if choice.Label != "" {
		*status = statusChoice(choice.Label)
	} else {
		*status = statusChoice(choice.Value)
	}
	return nil
}

func (status statusChoice) text() string {
	return string(status)
}

type ipAddress struct {
	Address string `json:"address"`
}

func (record serviceRecord) parentURL() string {
	if record.Parent.URL != "" {
		return record.Parent.URL
	}
	if record.Device.URL != "" {
		return record.Device.URL
	}
	return record.VirtualMachine.URL
}

func (record serviceRecord) toService() Service {
	service := Service{Device: record.Device.Name, VM: record.VirtualMachine.Name, Name: record.Name, Protocol: string(record.Protocol), Ports: record.Ports}
	if service.TargetName() == "" {
		parentName := record.Parent.Name
		if parentName == "" {
			parentName = record.Parent.Display
		}
		if strings.Contains(record.Parent.ObjectType, "virtualmachine") {
			service.VM = parentName
		} else {
			service.Device = parentName
		}
	}
	for _, address := range record.IPAddresses {
		service.IPs = append(service.IPs, address.Address)
	}
	for _, tag := range record.Tags {
		service.Tags = append(service.Tags, tag.text())
	}
	return service
}
