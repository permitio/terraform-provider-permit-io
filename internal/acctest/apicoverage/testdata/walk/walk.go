// Package walk has a call of each kind TestWalkPackage checks: an SDK Api method
// called directly, one used as a method value, and a request built with net/http.
package walk

import (
	"context"
	"net/http"

	"github.com/permitio/permit-golang/pkg/permit"
)

func read(ctx context.Context, c *permit.Client) error {
	_, err := c.Api.Tenants.Get(ctx, "tenant")
	return err
}

func remove(ctx context.Context, c *permit.Client) error {
	deleteRelation := c.Api.ResourceRelations.Delete
	return deleteRelation(ctx, "resource", "relation")
}

func send(ctx context.Context) (*http.Request, error) {
	return http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost/v2/things", nil)
}
