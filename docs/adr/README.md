# Architecture Decision Records

This directory contains Architecture Decision Records (ADRs) for agentpack. ADRs
capture significant design decisions along with their context and consequences.

## Format

ADRs follow [MADR 4.0.0](https://adr.github.io/madr/) (Markdown Any Decision
Records). Each ADR has YAML frontmatter (`status`, `date`) followed by these
sections:

| Section                           | Purpose                                               | Required |
| --------------------------------- | ----------------------------------------------------- | -------- |
| **Title** (h1)                    | Short noun phrase: ADR-NNN: Decision Title            | yes      |
| **Context and Problem Statement** | Forces at play, the problem being solved              | yes      |
| **Decision Drivers**              | Constraints and requirements that shaped the decision | optional |
| **Considered Options**            | What else was evaluated and why it was rejected       | yes      |
| **Decision Outcome**              | The change being made and rationale                   | yes      |
| **Consequences**                  | What happens after — positive, negative, neutral      | optional |
| **More Information**              | Links, references, related ADRs                       | optional |

## Conventions

- Number sequentially: `001-`, `002-`, etc.
- File names are kebab-case: `003-dependency-management.md`
- ADRs are point-in-time records — don't rewrite accepted ADRs. When a decision
  is reversed, write a new ADR and mark the old one as superseded in its
  frontmatter.
- Write a new ADR when changing: archive format, CLI vocabulary, target
  structure, dependency management model, or security policy.

## Index

| ADR                                    | Title                                   | Status                      |
| -------------------------------------- | --------------------------------------- | --------------------------- |
| [001](001-agentpack-format.md)         | .agentpack Package Format               | Superseded by 009           |
| [002](002-cli-vocabulary.md)           | CLI Command Vocabulary                  | Partially superseded by 003 |
| [003](003-dependency-management.md)    | Dependency Management Architecture      | Accepted                    |
| [004](004-config-merging.md)           | Config Merging for MCP, Hooks, Settings | Accepted                    |
| [005](005-content-safety.md)           | Content Safety and Executable Prompts   | Accepted                    |
| [006](006-agent-registry-alignment.md) | Agent Registry Alignment                | Accepted                    |
| [007](007-skill-search.md)             | Skill Search and Discovery              | Accepted                    |
| [008](008-content-filtering.md)        | Content Type Filtering in Manifest      | Superseded by 010           |
| [009](009-metadata-driven-format.md)   | Metadata-Driven Package Format          | Accepted                    |
| [010](010-content-selectors.md)        | Content Selectors and Version Pinning   | Accepted                    |
