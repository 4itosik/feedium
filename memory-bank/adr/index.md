---
doc_kind: adr
doc_function: index
purpose: Навигация по ADR проекта. Читать, чтобы найти уже принятые решения или завести новый ADR по шаблону.
derived_from:
  - ../dna/governance.md
status: active
---

# Architecture Decision Records Index

Каталог `memory-bank/adr/` хранит instantiated ADR проекта.

- Держи в этом каталоге только реальные decision records, а не заметки или черновые исследования.
- Если ADR пока нет, этот индекс остается пустым и служит ожидаемой точкой размещения для будущих решений.


## Naming

- Формат файла: `ADR-XXX-short-decision-name.md`
- Нумерация монотонная и не переиспользуется
- Заголовок файла должен совпадать с `title` во frontmatter

## Statuses

- `proposed` — решение сформулировано, но еще не принято
- `accepted` — решение принято и считается canonical input для downstream-документов
- `superseded` — решение заменено другим ADR
- `rejected` — решение рассмотрено и отклонено


## ADR Templates

````markdown
---
doc_kind: adr
doc_function: template
purpose: Governed wrapper-шаблон ADR. Читать, чтобы инстанцировать decision record без смешения metadata wrapper-документа и frontmatter будущего ADR.
derived_from:
  - ../dna/governance.md
  - ../dna/frontmatter.md
status: active
---

# ADR-XXX: Short Decision Name

Этот файл описывает wrapper-template. Инстанцируемый ADR живет ниже как embedded contract и копируется без wrapper frontmatter и history.

## Wrapper Notes

`decision_status: proposed` в embedded contract ниже означает, что текст ADR является предложением и не считается принятым решением до перевода инстанцированного ADR в статус `accepted`.

## Instantiated Frontmatter

```yaml
doc_kind: adr
doc_function: canonical
purpose: "Фиксирует архитектурное или инженерное решение, его текущий `decision_status` и последствия."
derived_from:
  # - ../features/FT-XXX/spec.md  # раскомментировать когда появится feature package
status: draft
decision_status: proposed
date: YYYY-MM-DD
```

## Instantiated Body

```markdown
# ADR-XXX: Short Decision Name

## Контекст

Какую проблему, ограничение, trade-off или архитектурное напряжение нужно разрешить.

## Драйверы решения

- какие требования или ограничения влияют на выбор;
- какие KPI, эксплуатационные или продуктовые факторы важны;
- какие зависимости и уже принятые решения нужно учитывать.

## Рассмотренные варианты

| Вариант | Плюсы | Минусы | Почему рассматривается как основной кандидат / не основной кандидат |
| --- | --- | --- | --- |
| `Option A` | Что дает | Какие ограничения создает | Причина решения |

## Решение

Для `decision_status: proposed` опиши здесь предлагаемое решение и избегай языка финального выбора (`выбрано`, `окончательно отвергнуто`, `принято`) до перевода ADR в `accepted`. После перевода ADR в `accepted` обнови формулировки так, чтобы секция фиксировала уже принятое решение, его границы действия и затронутые компоненты.

## Последствия

### Положительные

Что упрощается, улучшается или становится возможным.

### Отрицательные

Какие ограничения, долги или дополнительные издержки появляются.

### Нейтральные / организационные

Какие документы, процессы или зоны ответственности нужно обновить после принятия.

## Риски и mitigation

Какие риски остаются после выбора и как мы их снижаем.

## Follow-up

Какие downstream-документы, задачи, бенчмарки или миграции должны последовать за этим решением.

## Связанные ссылки

- feature / spec / analysis документы, которые дают контекст;
- связанные ADR, если решение зависит от них или уточняет их.
````