terraform {
  required_providers {
    permitio = {
      source = "registry.terraform.io/permitio/permit-io"
    }
  }
}

provider "permitio" {
  api_key = "permit_key_YzqvnAxYJgsAyAQUNm17Vg4lITVTC9h5pZd7JU5NmohY2uDGcp1xfCEcxEjHBAxnCnE1zNHBDQiG4bsOMXs5gi"
}

#data "permitio_resource" "wow" {
#  id = "1098f0f1360d4e76bfee159aff20c487"
#}

resource "permitio_resource" "wowa" {
  key         = "wowazaa"
  name        = "wowazaa"
  description = "wosdsfwazaa"
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

output "my_resource" {
  value = permitio_resource.wowa
}