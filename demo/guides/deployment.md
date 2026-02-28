# Deployment

sitegen can be deployed as a container, on Kubernetes, or as a static site on GitHub Pages.

## Docker

```bash
docker run --rm \
  -v ./content:/content:ro \
  -p 8080:8080 \
  ghcr.io/qrkourier/sitegen:latest \
  serve -src /content -out /docs -addr :8080
```

## Docker Compose

```bash
docker compose up
```

Docker Compose loads `.env` automatically for optional TLS and OpenZiti credentials.

## Kubernetes

Kustomize-ready manifests are provided:

```bash
kubectl create configmap sitegen-content \
  --from-file=content/ -n sitegen
kubectl apply -k deploy/kubernetes/
```

## GitHub Pages

The repository includes a GitHub Actions workflow that builds the site and deploys to Pages on every push to `main`.

## Optional TLS

Set three environment variables to enable automatic ACME TLS:

| Variable | Purpose |
|---|---|
| `DNS_SAN` | Domain name for the certificate |
| `CLOUDFLARE_API_KEY` | Cloudflare API token (DNS edit) |
| `TLS_PRIVKEY` | Base64-encoded PEM private key |

The certificate is cached in `cert.pem` and renewed automatically when within 30 days of expiry.
