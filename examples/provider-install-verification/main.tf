terraform {
  required_providers {
    permitio = {
      source = "registry.terraform.io/permitio/permit-io"
    }
  }
}

provider "permitio" {
    api_key = "permit_key_xxx"

}

data "permitio_resource" "wow" {
  id = "1098f0f1360d4e76bfee159aff20c487"
}

output "my_resource" {
  value = data.permitio_resource.wow
}