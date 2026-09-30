# Import a tenant by its key.
# An imported tenant with no attributes has none in the state, so if the
# configuration sets attributes = jsonencode({}), the first plan after the import
# adds it in place. Applying that plan changes nothing in Permit.
terraform import permitio_tenant.example acme
