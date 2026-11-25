package servicedeskprofile

import (
	"context"
	"net/http"

	"github.com/serialq7ic4/ixf-toolbox/internal/servicedeskapi"
)

type APIError = servicedeskapi.APIError
type Service = servicedeskapi.Service
type Lading = servicedeskapi.Lading

type MetadataSource interface {
	Service(context.Context, EditTarget) (Service, error)
	Lading(context.Context, EditTarget, string) (Lading, error)
}

type ClientOptions struct {
	HTTPClient *http.Client
	Transport  http.RoundTripper
}

type MetadataClient struct {
	client *servicedeskapi.Client
}

func NewMetadataClient(apiKey string, options ClientOptions) *MetadataClient {
	return &MetadataClient{client: servicedeskapi.NewClient(apiKey, servicedeskapi.Options{
		HTTPClient: options.HTTPClient,
		Transport:  options.Transport,
	})}
}

func (c *MetadataClient) Service(ctx context.Context, target EditTarget) (Service, error) {
	return c.client.Service(ctx, target.BusinessID, target.ServiceID)
}

func (c *MetadataClient) Lading(ctx context.Context, target EditTarget, snapshotID string) (Lading, error) {
	return c.client.Lading(ctx, target.BusinessID, target.ServiceID, snapshotID)
}

func invalidRequest(message string) *APIError {
	return servicedeskapi.InvalidRequest(message)
}
