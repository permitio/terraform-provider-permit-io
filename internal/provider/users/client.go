package users

import (
	"context"
	"github.com/permitio/permit-golang/pkg/permit"
)

type userClient struct {
	client *permit.Client
}

func (c *userClient) Read(ctx context.Context, key string) (userModel, error) {
	userRead, err := c.client.Api.Users.Get(ctx, key)
	if err != nil {
		return userModel{}, err
	}
	return tfModelFromUserRead(*userRead), nil
}
