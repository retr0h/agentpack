---
name: tf-review
description: Review Terraform plans for risky changes
---

# Terraform Plan Review

Flag these as risky:

- Any `destroy` action on production resources
- Changes to IAM policies or roles
- Security group rule modifications
- Database instance type changes
- Changes to encryption settings
