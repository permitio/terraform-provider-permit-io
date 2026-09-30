# Import a proxy config by its key. The import reads the secret back from the
# Permit API, and the mapping rules in the order Permit holds them. If the
# configuration lists them in another order, the next apply sends them in the
# configuration's order.
terraform import permitio_proxy_config.example billing
