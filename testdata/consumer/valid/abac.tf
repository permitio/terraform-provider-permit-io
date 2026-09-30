resource "permitio_user_set" "engineering" {
  key         = "engineering"
  name        = "Engineering"
  description = "Users in the engineering department"
  conditions = jsonencode({
    allOf = [
      { allOf = [{ "subject.department" = { equals = "engineering" } }] },
    ]
  })
}

resource "permitio_resource_set" "published_documents" {
  key         = "published_documents"
  name        = "Published documents"
  description = "Documents that are published"
  resource    = permitio_resource.document.key
  conditions = jsonencode({
    allOf = [
      { allOf = [{ "resource.published" = { equals = true } }] },
    ]
  })
}

resource "permitio_condition_set_rule" "engineering_reads_published" {
  user_set     = permitio_user_set.engineering.key
  resource_set = permitio_resource_set.published_documents.key
  permission   = "document:read"
}
