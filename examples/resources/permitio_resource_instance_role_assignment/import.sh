# Import a resource instance role assignment using the format:
# user:role:resource:resource_instance:tenant
# A key that contains ":" cannot be imported.
terraform import permitio_resource_instance_role_assignment.example john@example.com:viewer:document:doc-1:default
