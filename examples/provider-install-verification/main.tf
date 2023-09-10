terraform {
  required_providers {
    permitio = {
      source = "registry.terraform.io/permitio/permit-io"
    }
  }
}

provider "permitio" {
  api_key = ""
}

data "permitio_resource" "wow" {
  id = "1098f0f1360d4e76bfee159aff20c487"
}

resource "permitio_resource" "wowa" {
  key         = "wowazaa"
  name        = "wowazaa"
  urn = "urn:permitio:resource:1234"
  actions     = {
    read  = {
      name = "read"
      description = "asdfasdf"
    }
    delete  = {
      name = "read"
      description = "asdfasdf"
    }

    write = {
      name = "write"
      description = "asdfasdassdfasdff"
    }
    remove = {
        name = "remove"
    }
  }


}

resource "permitio_role" "writer" {
  key         = "writer"
  name        = "writer"
  description = "admin"
  permissions = [
    "farm:set-on-fire"
  ]
  extends = [
    "admin"
  ]
}

output "my_resource" {
  value = permitio_role.writer
}