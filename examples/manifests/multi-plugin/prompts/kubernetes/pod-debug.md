---
name: pod-debug
description: Debug failing Kubernetes pods
---

# Pod Debugging

When a pod is failing, check in this order:

1. `kubectl describe pod` for events
2. `kubectl logs` for application errors
3. Resource limits and requests
4. Liveness/readiness probe configuration
5. Network policies blocking traffic
