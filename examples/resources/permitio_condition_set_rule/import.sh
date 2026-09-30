# Import a condition set rule using the format: user_set,permission,resource_set
# The permission is resource_key:action_key. A key that contains "," cannot be
# imported.
terraform import permitio_condition_set_rule.example admins,document:read,sensitive-docs
