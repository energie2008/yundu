# YunDu Helm Chart

One-click Kubernetes deployment for YunDu proxy panel.

## Quick Start

```bash
# Add required values
helm install yundu ./deploy/k8s/helm \
  --set nodeService.env.DB_HOST=your-db-host \
  --set nodeService.envSecrets.DB_PASSWORD=your-db-password \
  --set nodeService.env.REDIS_HOST=your-redis-host \
  --set nodeService.envSecrets.REDIS_PASSWORD=your-redis-password \
  --set ingress.hosts[0].host=admin.your-domain.com

# Production deployment
helm install yundu ./deploy/k8s/helm -f values-production.yaml \
  --set nodeService.env.DB_HOST=your-db-host \
  --set nodeService.envSecrets.DB_PASSWORD=your-db-password
```

## Architecture

- **node-service**: Panel backend (Deployment, HPA-enabled in production)
- **admin-web**: Frontend SPA (Deployment, multi-replica)
- **node-agent**: Edge proxy agent (DaemonSet on labeled nodes)

## Standalone Mode

For single-node deployment without panel:

```bash
helm install yundu ./deploy/k8s/helm \
  --set nodeService.enabled=false \
  --set adminWeb.enabled=false \
  --set nodeAgent.enabled=true \
  --set nodeAgent.standalone.enabled=true \
  --set nodeAgent.standalone.config='{"server_code":"edge-01","nodes":[...]}'
```
