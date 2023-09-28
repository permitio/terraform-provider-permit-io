package proxy_configs

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/permitio/permit-golang/pkg/permit"
)

var (
	_ resource.Resource              = &proxyConfigResource{}
	_ resource.ResourceWithConfigure = &proxyConfigResource{}
)

func NewProxyConfigResource() resource.Resource {
	return &proxyConfigResource{}
}

type proxyConfigResource struct {
	client          ProxyConfigClient
	proxyConfigType models.ProxyConfigType
}

func (c *proxyConfigResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_proxy_config"
}

func (c *proxyConfigResource) Configure(_ context.Context, request resource.ConfigureRequest, response *resource.ConfigureResponse) {
	if request.ProviderData == nil {
		return
	}

	permitClient, ok := request.ProviderData.(*permit.Client)

	if !ok {
		response.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *permit.Client, got: %T. Please report this issue to the provider developers.", request.ProviderData),
		)
	}

	c.client = ProxyConfigClient{client: permitClient}

	return
}

func (c *proxyConfigResource) Schema() *schema.Resource {
	return &schema.Resource{
		Schema: map[string]*schema.Schema{
			"": {},
		},
	}
}

func (c *proxyConfigResource) Read(_ context.Context, request resource.ReadRequest, response *resource.ReadResponse) {
	proxyConfig, err := c.client.ReadProxyConfig(request.ID)

	if err != nil {
		response.Diagnostics.AddError(
			"Error reading proxy config",
			fmt.Sprintf("Error reading proxy config: %s", err),
		)
		return
	}

	response.State = proxyConfig
}

func (c *proxyConfigResource) Create(_ context.Context, request resource.CreateRequest, response *resource.CreateResponse) {
	proxyConfig, err := c.client.CreateProxyConfig(request.Config)

	if err != nil {
		response.Diagnostics.AddError(
			"Error creating proxy config",
			fmt.Sprintf("Error creating proxy config: %s", err),
		)
		return
	}

	response.State = proxyConfig
}

func (c *proxyConfigResource) Update(_ context.Context, request resource.UpdateRequest, response *resource.UpdateResponse) {
	proxyConfig, err := c.client.UpdateProxyConfig(request.ID, request.Patch)

	if err != nil {
		response.Diagnostics.AddError(
			"Error updating proxy config",
			fmt.Sprintf("Error updating proxy config: %s", err),
		)
		return
	}

	response.State = proxyConfig
}

func (c *proxyConfigResource) Delete(_ context.Context, request resource.DeleteRequest, response *resource.DeleteResponse) {
	err := c.client.DeleteProxyConfig(request.ID)

	if err != nil {
		response.Diagnostics.AddError(
			"Error deleting proxy config",
			fmt.Sprintf("Error deleting proxy config: %s", err),
		)
		return
	}

}
