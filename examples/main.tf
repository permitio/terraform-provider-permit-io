terraform {
  required_providers {
    permitio = {
      source = "registry.terraform.io/permitio/permit-io"
    }
  }
}

provider "permitio" {
  api_key = "SET ENV - PERMITIO_API_KEY"
}


resource "permitio_resource" "document" {
  key         = "document"
  name        = "document"
  description = "a new document"
  actions = {
    "read" = {
      "name" = "read"
    }
    "write" = {
      "name" = "write"
    }
    "delete" = {
      "name"        = "write"
      "description" = "delete a document"
    }
  }
  attributes = {
    "title" = {
      "description" = "the title of the document"
      "type"        = "string"
    }
  }
}

resource "permitio_role" "writer" {
  key         = "writer"
  name        = "writer"
  description = "a new admin"
  permissions = ["document:read", "document:write", "document:delete"]
  depends_on = [
    "permitio_resource.document"
  ]
}
resource "permitio_role" "admin" {
  key         = "admin"
  name        = "admin"
  description = "a new admin"
  permissions = ["document:read", "document:write"]
  extends     = []
  depends_on = [
    "permitio_resource.document",
    "permitio_role.writer"
  ]
}

resource "permitio_user_set" "privileged_users" {
  key  = "privileged_users"
  name = "Privileged Users"
  conditions = jsonencode({
    "allOf" : [
      {
        "allOf" : [
          {
            "subject.email" = {
              contains = "@admin.com"
            },
          }
        ]
      }
    ]
  })
}

resource "permitio_user_set" "unprivileged_users" {
  key  = "unprivileged_users"
  name = "Unprivileged Users"
  conditions = jsonencode({
    "allOf" : [
      {
        "allOf" : [
          {
            "subject.email" = {
              contains = "@user.com"
            },
          }
        ]
      }
    ]
  })
}

resource "permitio_resource_set" "secret_docs" {
  key      = "secret_docs"
  name     = "Secret Docs"
  resource = permitio_resource.document.key
  conditions = jsonencode({
    "allOf" : [
      {
        "allOf" : [
          {
            "resource.title" = {
              contains = "Rye"
            },
          }
        ]
      }
    ]
  })
}

resource "permitio_condition_set_rule" "allow_privileged_users_to_read_secret_docs" {
  user_set     = permitio_user_set.privileged_users.key
  resource_set = permitio_resource_set.secret_docs.key
  permission   = "document:read"
}

resource "permitio_proxy_config" "foaz" {
  key            = "foaz"
  name           = "Boaz"
  auth_mechanism = "Basic"
  auth_secret = {
    basic = "hello:world"
  }
  mapping_rules = [
    {
      url         = "https://example.com/documents"
      http_method = "post"
      resource    = "document"
      action      = "read"
    },
    {
      url         = "https://example.com/documents/{project_id}"
      http_method = "get"
      resource    = "document"
      action      = "read"
    },
    {
      url         = "https://example.com/documents/{project_id}"
      http_method = "put"
      resource    = "document"
      action      = "update"
      headers = {
        "x-update-id" : "foaz"
      }
    },
    {
      url         = "https://example.com/documents/{project_id}"
      http_method = "delete"
      resource    = "document"
      action      = "delete"
    }
  ]
}

output "my_resource" {
  value = permitio_role.admin
}