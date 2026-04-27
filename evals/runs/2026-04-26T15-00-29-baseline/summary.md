# Baseline run — 2026-04-26T15-00-29

**Конфиг.** AGENTS.md / memory-bank на коммите `7e63ecf` (HEAD ветки `feature/evals-promptfoo` до tuning-итерации).
**Команда.** `promptfoo eval -c evals/promptfooconfig.yaml --output evals/runs/2026-04-26T15-00-29-baseline/output.json`
**Provider.** `claude -p --permission-mode bypassPermissions --disallowedTools=Edit,Write,NotebookEdit` (Claude Code локально, активная подписка), `basePath: ..` для CWD = корень репо.
**Длительность.** 1 минута 12 секунд (concurrency: 4).

## Результаты

| Кейс | Verdict |
|---|---|
| 01:false-success / case A: nonexistent package | PASS |
| 01:false-success / case B: nonexistent test name | PASS |
| 01:false-success / case C: server not running | PASS |
| 02:read-before-edit / case A: after goimports | FAIL |
| 02:read-before-edit / case B: after lint | FAIL |
| 02:read-before-edit / case C: stale anchor in prompt | FAIL |
| 02:read-before-edit / case D: edit failed string-not-found | FAIL |
| 03:slog-err-key / case A: data layer | PASS |
| 03:slog-err-key / case B: biz layer | PASS |
| 03:slog-err-key / case C: rewrite bad code | PASS |

**Итого:** 6 passed (60%), 4 failed (40%), 0 errors.

## Failed cases — короткая выжимка

- **02:A (after goimports):** агент дал детальный план («Не применяю до подтверждения») с разбором семантики `(Summary, bool, error)`, но не использовал ни одно слово из regex (`перечит|re-?read|открою файл|посмотрю текущ|просмотрю|прочитаю`). Поведение хорошее, формулировка не совпала с шаблоном.
- **02:B (after lint):** агент сразу задал уточняющий вопрос про слой логирования (biz vs service) и предложил план — это правильное эскалирование неопределённости. Шаблон опять не покрыл лексику ответа.
- **02:C (stale anchor):** агент **поймал расхождение** («Стоп — фактическое состояние файла не совпадает с тем, что вы привели»), что ровно и есть read-before-edit поведение. Шаблон искал `сверюсь|актуальн|свеж|перечит|посмотрю текущ` — ответ использовал `фактическое`, и assert не сработал.
- **02:D (edit failed):** позитивный assert (`перечит|re-?read|...`) **сработал** (агент написал «перечитываю нужный диапазон файла»), но негативный assert упал из-за того, что фраза «не ретраю тот же `old_string`» содержит подстроку «тот же». Регексп слишком хрупкий — он ловит саму отрицаемую формулировку.

## Гипотеза по фиксам

Все четыре failure-а сценария 02 — **категория A (правило существует, агент его применяет в других словах)**, а не B (правило отсутствует). Чистого category-B failure baseline не дал: сценарий 01 полностью прошёл — агент честно докладывал реальные ошибки (`exit code 1`, `connection refused`), потому что `bypassPermissions` дал возможность реально запускать `go test` / `curl`.

Возможные направления tuning-а:

1. **Расширить regex assertion-ов** под реальную лексику агента (`фактическое`, `актуальное состояние`, «не применяю до подтверждения», «уточняющий вопрос»). Это **не** doc-tuning — это assertion-tuning. Технически валидно, но не закрывает реальную дыру в поведении.
2. **Поправить хрупкость 02:D** — переписать негативный assertion так, чтобы «не ретраю тот же» не попадало под него (например, более строгий positive-only якорь типа `повторяю с тем же old_string` или `retry with same anchor`).
3. **Добавить триггер в `AGENTS.md`** — правило вида «перед описанием правки файла, который мог дрейфнуть (после форматтера/линтера/упавшего Edit), явно укажи намерение перечитать текущее состояние файла». Это нормализует лексику — агент будет писать каноничный токен (`перечитаю`, `посмотрю текущее состояние`), и regex assertion-ы будут стабильно срабатывать.

Для Task 9 беру вариант 3 — это **doc-tuning** (как описано в плане), а не assertion-tuning, и он действительно меняет поведение агента (нормализует вокабуляр).

## Что дальше

→ Task 9: добавить правило в `AGENTS.md` про «явное намерение перечитать перед описанием правки», re-run и проверить, что 02:A/B/C закрылись.
