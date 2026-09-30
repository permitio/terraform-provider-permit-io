package proxy_configs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/permitio/permit-golang/pkg/models"
	"github.com/permitio/permit-golang/pkg/permit"
	"github.com/permitio/terraform-provider-permit-io/internal/provider/common"
	"github.com/permitio/terraform-provider-permit-io/internal/provider/config"
)

type proxyConfigClient struct {
	client *permit.Client
	// api is the provider's connection to the Permit API, which update sends its
	// request through, since the SDK cannot send should_delete. It is nil when the
	// provider has not been configured.
	api *config.API
}

// errNotConfigured is the error of an update made before the provider was
// configured, when there is no API URL, API key or HTTP client to send it with.
var errNotConfigured = errors.New("the Permit.io provider is not configured, so there is " +
	"no API URL, API key or HTTP client to send the request with")

// errNoEnvironment is the error of an update made with an API key that is not
// scoped to one environment, which the proxy config path must name.
var errNoEnvironment = errors.New("the Permit.io API key is not scoped to an environment, " +
	"so there is no project and environment to send the proxy config update to; use an " +
	"environment API key")

func (c *proxyConfigClient) create(ctx context.Context, model proxyConfigModel) (proxyConfigModel, error) {
	proxyConfigCreate, err := model.toProxyConfigCreate(ctx)

	if err != nil {
		return proxyConfigModel{}, err
	}

	proxyConfig, err := c.client.Api.ProxyConfigs.Create(ctx, proxyConfigCreate)

	if err != nil {
		return proxyConfigModel{}, err
	}

	resultModel := proxyConfigModel{}
	resultModel.fromProxyConfigRead(proxyConfig)

	return resultModel, nil
}

// read returns the proxy config with its mapping rules in the order of the rules
// in model, which the API may hold in another order.
func (c *proxyConfigClient) read(ctx context.Context, model proxyConfigModel) (proxyConfigModel, error) {
	proxyConfig, err := c.client.Api.ProxyConfigs.Get(ctx, ident(model))

	if err != nil {
		return proxyConfigModel{}, err
	}

	resultModel := proxyConfigModel{}
	resultModel.fromProxyConfigRead(proxyConfig)
	resultModel.MappingRules = orderMappingRules(resultModel.MappingRules, model.MappingRules)

	return resultModel, nil
}

// update sends the planned proxy config to the API, with should_delete for each
// mapping rule of the prior state before the planned rules, and returns the result
// with its mapping rules in the planned order.
func (c *proxyConfigClient) update(
	ctx context.Context, plan, prior proxyConfigModel,
) (proxyConfigModel, error) {
	if c.api == nil || c.api.HTTPClient == nil {
		return proxyConfigModel{}, errNotConfigured
	}
	if c.api.ProjectID == "" || c.api.EnvironmentID == "" {
		return proxyConfigModel{}, errNoEnvironment
	}
	patch, err := plan.toProxyConfigPatch(ctx, prior)
	if err != nil {
		return proxyConfigModel{}, err
	}
	body, err := json.Marshal(patch)
	if err != nil {
		return proxyConfigModel{}, fmt.Errorf("encoding the proxy config update: %w", err)
	}
	configURL := fmt.Sprintf("%s/v2/facts/%s/%s/proxy_configs/%s",
		strings.TrimSuffix(c.api.URL, "/"), url.PathEscape(c.api.ProjectID),
		url.PathEscape(c.api.EnvironmentID), url.PathEscape(ident(plan)))

	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, configURL, bytes.NewReader(body))
	if err != nil {
		return proxyConfigModel{}, fmt.Errorf("creating the proxy config update request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.api.Key)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.api.HTTPClient.Do(req)
	if err != nil {
		return proxyConfigModel{}, fmt.Errorf("sending the proxy config update: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return proxyConfigModel{}, fmt.Errorf("reading the proxy config update response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return proxyConfigModel{}, &common.APIStatusError{
			StatusCode: resp.StatusCode, Body: string(respBody),
		}
	}

	// The body holds the secret, so the error does not quote it.
	var proxyConfig models.ProxyConfigRead
	if err := json.Unmarshal(respBody, &proxyConfig); err != nil {
		return proxyConfigModel{}, fmt.Errorf("decoding the updated proxy config: %w", err)
	}
	resultModel := proxyConfigModel{}
	resultModel.fromProxyConfigRead(&proxyConfig)
	resultModel.MappingRules = orderMappingRules(resultModel.MappingRules, plan.MappingRules)

	return resultModel, nil
}

func (c *proxyConfigClient) delete(ctx context.Context, model proxyConfigModel) error {
	return c.client.Api.ProxyConfigs.Delete(ctx, ident(model))
}

func ident(model proxyConfigModel) string {
	if model.Key.IsNull() {
		return model.Id.ValueString()
	} else {
		return model.Key.ValueString()
	}
}
