# Architecture Decision Records

This directory contains Architecture Decision Records (ADRs) for agentpack. ADRs
capture significant design decisions along with their context and consequences.

## Format

ADRs follow [MADR](https://adr.github.io/madr/) (Markdown Any Decision Records),
an extension of Michael Nygard's original format:

| Section                     | Purpose                                                                | Required |
| --------------------------- | ---------------------------------------------------------------------- | -------- |
| **Title**                   | Short noun phrase: ADR-NNN: Decision Title                             | yes      |
| **Status**                  | Proposed, Accepted, Deprecated, or Superseded by ADR-NNN               | yes      |
| **Decision Drivers**        | Constraints and requirements that shaped the decision                  | yes      |
| **Considered Alternatives** | What else was evaluated and why it was rejected                        | yes      |
| **Context**                 | Forces at play, including technical, political, and project-local      | yes      |
| **Decision**                | The change being made                                                  | yes      |
| **Consequences**            | What happens after applying the decision — positive, negative, neutral | yes      |

Additional sections (e.g. **Influences**) are permitted when they add clarity.

## Conventions

- Number sequentially: `001-`, `002-`, etc.
- File names are kebab-case: `003-dependency-management.md`
- ADRs are point-in-time records — don't rewrite accepted ADRs. When a decision
  is reversed, write a new ADR and mark the old one as superseded in its Status
  section.
- Write a new ADR when changing: archive format, CLI vocabulary, target
  structure, dependency management model, or security policy.

## Index

| ADR                                 | Title                                   | Status                                 |
| ----------------------------------- | --------------------------------------- | -------------------------------------- |
| [001](001-agentpack-format.md)      | .agentpack Package Format               | Accepted                               |
| [002](002-cli-vocabulary.md)        | CLI Command Vocabulary                  | Accepted (partially superseded by 003) |
| [003](003-dependency-management.md) | Dependency Management Architecture      | Accepted                               |
| [004](004-config-merging.md)        | Config Merging for MCP, Hooks, Settings | Accepted                               |
