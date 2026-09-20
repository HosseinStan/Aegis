# Deployment

The root `compose.yaml` runs Aegis API without a host port and places Caddy in
front of it for HTTPS. Production operators must replace the local internal-CA
certificate setup with the organization's approved DNS and certificate
configuration, protect the persistent volume, and restrict the management
network.

