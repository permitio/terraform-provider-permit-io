package common

import (
	"context"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/permitio/permit-golang/pkg/permit"
)

func Configure(_ context.Context, request resource.ConfigureRequest, response *resource.ConfigureResponse) *permit.Client {
	if request.ProviderData == nil {
		return nil
	}

	permitClient, ok := request.ProviderData.(*permit.Client)

	if !ok {
		response.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *permit.Client, got: %T. Please report this issue to the provider developers.", request.ProviderData),
		)
		return nil
	}

	return permitClient
}
