terraform {
  required_providers {
    permitio = {
      source = "permitio/permit-io"
    }
  }
}

resource "permitio_resource" "document" {
  key  = "document"
  name = "Document"
  actions = {
    read = { name = "Read" }
  }
  attributes = {
    classified = { type = "bool" }
  }
}

resource "permitio_user_set" "security_team" {
  key  = "security_team"
  name = "Security team"
  conditions = jsonencode({
    allOf = [
      { allOf = [{ "subject.email" = { contains = "@security.example.com" } }] },
    ]
  })
}

resource "permitio_resource_set" "classified_documents" {
  key      = "classified_documents"
  name     = "Classified documents"
  resource = permitio_resource.document.key
  conditions = jsonencode({
    allOf = [
      { allOf = [{ "resource.classified" = { equals = true } }] },
    ]
  })
}

# The security team may read classified documents.
resource "permitio_condition_set_rule" "security_team_reads_classified" {
  user_set     = permitio_user_set.security_team.key
  permission   = "${permitio_resource.document.key}:read"
  resource_set = permitio_resource_set.classified_documents.key
}
