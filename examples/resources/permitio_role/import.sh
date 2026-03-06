# Import a top-level role using the format: role_key
terraform import permitio_role.example admin

# Import a resource-level role using the format: resource_key:role_key
terraform import permitio_role.example document:editor
