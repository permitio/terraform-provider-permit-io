package mockpermit

import (
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// CheckRequests returns a Terraform test check that passes when the requests the
// fake has received so far with this method and path have exactly the JSON bodies
// in want, in any order. With no want it checks that no such request was sent.
func (s *Server) CheckRequests(method, urlPath string, want ...string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		requests := s.Requests(method, urlPath)
		if len(requests) != len(want) {
			return fmt.Errorf("%s %s: got %d requests, want %d; bodies: %s",
				method, urlPath, len(requests), len(want), bodies(requests))
		}
		matched := make([]bool, len(requests))
		for _, body := range want {
			found := false
			for i, request := range requests {
				if !matched[i] && request.CheckJSONBody(body) == nil {
					matched[i], found = true, true
					break
				}
			}
			if !found {
				return fmt.Errorf("%s %s: no request has the body %s; bodies: %s",
					method, urlPath, body, bodies(requests))
			}
		}
		return nil
	}
}

// CheckEmpty is a CheckDestroy function that fails while the fake still holds an
// object, such as one a destroy left behind.
func (s *Server) CheckEmpty(*terraform.State) error {
	if keys := s.StoredKeys(); len(keys) > 0 {
		return fmt.Errorf("objects left in the mock after destroy: %q", keys)
	}
	return nil
}

func bodies(requests []Request) string {
	var all []string
	for _, request := range requests {
		all = append(all, string(request.Body))
	}
	return "[" + strings.Join(all, ", ") + "]"
}
