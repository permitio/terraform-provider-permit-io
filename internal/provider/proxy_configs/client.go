package proxy_configs

import (
	"context"
	"github.com/permitio/permit-golang/pkg/permit"
)

type proxyConfigClient struct {
	client *permit.Client
}

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

func (c *proxyConfigClient) read(ctx context.Context, model proxyConfigModel) (proxyConfigModel, error) {
	proxyConfig, err := c.client.Api.ProxyConfigs.Get(ctx, ident(model))

	if err != nil {
		return proxyConfigModel{}, err
	}

	resultModel := proxyConfigModel{}
	resultModel.fromProxyConfigRead(proxyConfig)

	return resultModel, nil
}

func (c *proxyConfigClient) update(ctx context.Context, model proxyConfigModel) (proxyConfigModel, error) {
	proxyConfigUpdate, err := model.toProxyConfigUpdate(ctx)

	if err != nil {
		return proxyConfigModel{}, err
	}

	proxyConfig, err := c.client.Api.ProxyConfigs.Update(ctx, ident(model), proxyConfigUpdate)

	if err != nil {
		return proxyConfigModel{}, err
	}

	resultModel := proxyConfigModel{}
	resultModel.fromProxyConfigRead(proxyConfig)

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
