terraform {
  required_providers {
    permitio = {
      source = "permitio/permit-io"
    }
  }
}

# A user set and a resource set that are managed outside this configuration.
data "permitio_condition_set" "finance" {
  key = "finance"
}

data "permitio_condition_set" "invoices" {
  key = "invoices"
}

# Finance users may read the invoices in the invoices resource set.
resource "permitio_condition_set_rule" "finance_reads_invoices" {
  user_set     = data.permitio_condition_set.finance.key
  permission   = "${data.permitio_condition_set.invoices.resource}:read"
  resource_set = data.permitio_condition_set.invoices.key
}
