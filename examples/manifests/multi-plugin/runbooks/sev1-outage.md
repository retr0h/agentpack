---
name: sev1-outage
description: SEV1 outage response runbook
---

# SEV1 Outage Response

1. **Acknowledge** the incident in PagerDuty within 5 minutes
2. **Assemble** the incident channel — page SRE lead and service owner
3. **Diagnose** — check dashboards, logs, recent deploys
4. **Mitigate** — rollback, feature flag, or scale up
5. **Communicate** — update status page every 15 minutes
6. **Resolve** — confirm metrics are nominal for 30 minutes
7. **Postmortem** — schedule within 48 hours
