# Import a resource instance using the format: resource_key:instance_key
# The ID is split at its first ":", so the instance key may contain ":".
# An imported instance with no attributes has none in the state, so if the
# configuration sets attributes = jsonencode({}), the first plan after the import
# adds it in place. Applying that plan changes nothing in Permit.
terraform import permitio_resource_instance.example document:doc-1
