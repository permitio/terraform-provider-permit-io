# Import a group resource instance role assignment using the format:
# group:role:resource:resource_instance:tenant
# A key that contains ":" cannot be imported.
terraform import permitio_group_resource_instance_role_assignment.example engineering:viewer:document:doc-1:default
