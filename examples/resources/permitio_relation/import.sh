# Import a relation using the format: object_resource:key
# object_resource is the key, not the ID, of the resource the relation is on. A key
# that contains ":" cannot be imported.
terraform import permitio_relation.example file:parent
