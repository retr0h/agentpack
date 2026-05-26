# Architecture Decision Records

This directory contains Architecture Decision Records (ADRs) for agentpack. ADRs
capture significant design decisions along with their context and consequences.

## Format

ADRs follow [Michael Nygard's format](https://adr.github.io/):

| Section          | Purpose                                                                |
| ---------------- | ---------------------------------------------------------------------- |
| **Title**        | Short noun phrase: ADR-NNN: Decision Title                             |
| **Status**       | Proposed, Accepted, Deprecated, or Superseded by ADR-NNN               |
| **Context**      | Forces at play, including technical, political, and project-local      |
| **Decision**     | The change being made                                                  |
| **Consequences** | What happens after applying the decision — positive, negative, neutral |

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
| [003](003-dependency-management.md) | Dependency Management Architecture      | Proposed                               |
| [004](004-config-merging.md)        | Config Merging for MCP, Hooks, Settings | Proposed                               |
