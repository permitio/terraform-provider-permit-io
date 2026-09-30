package mockpermit

import (
	"encoding/json"
	"fmt"
	"slices"
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

// CheckStored returns a CheckDestroy function that fails unless the fake holds
// exactly the objects in want, written "collection/key" as StoredKeys lists them,
// such as the users a test added itself with AddUser.
func (s *Server) CheckStored(want ...string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		if keys := s.StoredKeys(); !slices.Equal(keys, slices.Sorted(slices.Values(want))) {
			return fmt.Errorf("objects in the mock after destroy: %q, want %q", keys, want)
		}
		return nil
	}
}

// CheckStoredJSON returns a Terraform test check that passes when the field of the
// stored object, written "collection/key" as StoredKeys lists it, encodes exactly
// as want: compact, with sorted object keys and each number as the request that
// set it wrote it.
func (s *Server) CheckStoredJSON(stored, field, want string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		collection, key, _ := strings.Cut(stored, "/")
		s.mu.Lock()
		object, ok := s.objects[collection][key]
		var value any
		if ok {
			value, ok = object[field]
		}
		s.mu.Unlock()
		if !ok {
			return fmt.Errorf("the mock holds no %s with the field %s; it holds %q",
				stored, field, s.StoredKeys())
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			return fmt.Errorf("encoding the %s of %s: %w", field, stored, err)
		}
		if string(encoded) != want {
			return fmt.Errorf("the %s of %s in the mock = %s, want %s", field, stored,
				encoded, want)
		}
		return nil
	}
}

func bodies(requests []Request) string {
	var all []string
	for _, request := range requests {
		all = append(all, string(request.Body))
	}
	return "[" + strings.Join(all, ", ") + "]"
}
