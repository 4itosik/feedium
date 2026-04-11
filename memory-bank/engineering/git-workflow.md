---
title: Git Workflow
doc_kind: engineering
doc_function: convention
purpose: Шаблон git workflow документа. После копирования зафиксируй реальные branch names, commit rules и PR expectations проекта.
derived_from:
  - ../dna/governance.md
status: active
audience: humans_and_agents
---

# Git Workflow

## Default Branch

Основной веткой является `main`

## Commits

## Commits

- Conventional Commits, English, present tense, <= 72 chars
- Сообщение должно объяснять *что* и при необходимости *зачем*  
  (для `fix:` и `refactor:` — обязательно указывать зачем)
- Каждый коммит с задачей (формат — в проекте)
- Разрешены `fixes`, `closes`, `resolves`
- Только rebase/squash, PR — squash merge

## Pull Requests

- Перед PR должны быть зелёными canonical local checks проекта
- PR title должен быть коротким и предметным
- В body полезно фиксировать: что изменено, зачем, как проверено, какие риски или manual steps остаются
