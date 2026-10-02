# Import a resource by its key.
# An imported resource with no attributes has none in the state, so if the
# configuration sets attributes = {}, the first plan after the import adds it in
# place. Applying that plan changes nothing in Permit.
terraform import permitio_resource.example document
