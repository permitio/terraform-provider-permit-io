# Import a role derivation using the format:
# resource:to_role:on_resource:role:linked_by
# For example, file:editor:folder:manager:parent is the derivation that makes
# managers of a folder editors of the files linked to it by the parent relation.
# Every part is a key, as in the configuration, which rejects IDs. A key that
# contains ":" cannot be imported.
terraform import permitio_role_derivation.example file:editor:folder:manager:parent
