---
name: database-failover
description: Database failover procedure
---

# Database Failover

1. Confirm the primary is truly unhealthy (not just slow queries)
2. Check replication lag on the replica
3. Promote the replica to primary
4. Update connection strings in the config service
5. Verify application connectivity
6. Monitor for data consistency issues
