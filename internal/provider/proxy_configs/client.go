package proxy_configs

import (
	"context"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/permitio/permit-golang/pkg/permit"
)

type ProxyConfigModel struct {
	Id             types.String `tfsdk:"id"`
	OrganizationId types.String `tfsdk:"organization_id"`
	ProjectId      types.String `tfsdk:"project_id"`
	EnvironmentId  types.String `tfsdk:"environment_id"`
	Key            types.String `tfsdk:"key"`
	Name           types.String `tfsdk:"name"`
	CreatedAt      types.String `tfsdk:"created_at"`
	UpdatedAt      types.String `tfsdk:"updated_at"`
	MappingRules   types.String `tfsdk:"mapping_rules"`
}
type ProxyConfigClient struct {
	client *permit.Client
}

func (c *ProxyConfigClient) Read(ctx context.Context, data ProxyConfigModel) (ProxyConfigModel, error) {

}

func (c *ProxyConfigClient) Create(ctx context.Context, data ProxyConfigModel) (ProxyConfigModel, error) {

}

func (c *ProxyConfigClient) Update(ctx context.Context, data ProxyConfigModel) (ProxyConfigModel, error) {

}

func (c *ProxyConfigClient) Delete(ctx context.Context, data ProxyConfigModel) (ProxyConfigModel, error) {

}
