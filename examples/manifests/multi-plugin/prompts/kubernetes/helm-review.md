---
name: helm-review
description: Review Helm charts for best practices
---

# Helm Chart Review

Check the chart for:

- Resource requests and limits on all containers
- Security contexts (non-root, read-only root filesystem)
- Proper use of ConfigMaps vs Secrets
- Health checks defined
- Horizontal pod autoscaler configured
