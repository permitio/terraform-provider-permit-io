# Import a role derivation using the format:
# resource:to_role:on_resource:role:linked_by
# For example, file:editor:folder:manager:parent is the derivation that makes
# managers of a folder editors of the files linked to it by the parent relation.
# The import keeps the resource part as given, so write it the way the
# configuration names the resource: its ID if the configuration uses the ID,
# such as permitio_resource.file.id, and its key otherwise. If they differ, the
# next plan replaces the derivation. The other parts are keys. A key that
# contains ":" cannot be imported.
terraform import permitio_role_derivation.example file:editor:folder:manager:parent
