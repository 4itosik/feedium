---
doc_kind: governance
doc_function: canonical
purpose: Визуализация дерева зависимостей (derived_from) всех governed-документов memory-bank. SSoT authority течёт сверху вниз.
derived_from:
  - governance.md
status: active
---

# Source Dependency Tree

Authority течёт **upstream → downstream** (сверху вниз).
Поле `derived_from` каждого документа определяет его прямой upstream.
Корень — `principles.md` (не имеет `derived_from`).

```
principles.md                          ← ROOT, не имеет derived_from
├── index.md                           (memory-bank root index)
├── governance.md
│   ├── frontmatter.md
│   │   └── (используется как upstream в шаблонах PRD/ADR)
│   ├── lifecycle.md
│   ├── domain/index.md
│   ├── domain/problem.md
│   │   └── prd/PRD-001-mvp.md
│   ├── domain/architecture.md
│   ├── domain/glossary.md
│   ├── dna/dependency-tree.md
│   ├── engineering/index.md
│   ├── engineering/coding-style.md
│   ├── engineering/api-contracts.md
│   ├── engineering/database.md
│   ├── engineering/testing-policy.md
│   ├── engineering/autonomy-boundaries.md
│   ├── engineering/git-workflow.md
│   ├── adr/index.md
│   └── prd/index.md
├── dna/index.md
└── cross-references.md
```

## Таблица зависимостей

| Документ | `derived_from` |
|---|---|
| `dna/principles.md` | — (root) |
| `index.md` | `dna/principles.md` |
| `dna/governance.md` | `principles.md` |
| `dna/frontmatter.md` | `governance.md` |
| `dna/lifecycle.md` | `governance.md` |
| `dna/index.md` | `principles.md` |
| `dna/cross-references.md` | `principles.md` |
| `domain/index.md` | `dna/governance.md` |
| `domain/problem.md` | `dna/governance.md` |
| `domain/architecture.md` | `dna/governance.md` |
| `domain/glossary.md` | `dna/governance.md` |
| `dna/dependency-tree.md` | `governance.md` |
| `engineering/index.md` | `dna/governance.md` |
| `engineering/coding-style.md` | `dna/governance.md` |
| `engineering/api-contracts.md` | `dna/governance.md` |
| `engineering/database.md` | `dna/governance.md` |
| `engineering/testing-policy.md` | `dna/governance.md` |
| `engineering/autonomy-boundaries.md` | `dna/governance.md` |
| `engineering/git-workflow.md` | `dna/governance.md` |
| `prd/index.md` | `dna/governance.md` |
| `prd/PRD-001-mvp.md` | `domain/problem.md` |
| `adr/index.md` | `dna/governance.md` |
