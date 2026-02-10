# Часть 3 — Конечный автомат (стейт-машина)

## Что должно быть готово после этого этапа

Вся бизнес-логика переходов между статусами задач, включая проверки прав, дедлайнов и зависимостей. Фоновый процесс проверки дедлайнов.

---

## 1. Статусы

| Статус | Описание | Имеет дедлайн |
|--------|----------|:--------------:|
| NEW | Создана, ожидает исполнителя или начала работы | Да |
| IN_PROGRESS | Исполнитель работает | Да |
| BLOCKED | Заблокирована эскалацией или вручную | Да |
| STUCK | Дедлайн истёк, нужно вмешательство | Нет |
| DONE | Завершена | Нет |
| CANCELLED | Отменена | Нет |

Терминальные статусы: DONE, CANCELLED — из них нет переходов.

---

## 2. Разрешённые переходы

| Из → В | Кто может | Условия |
|--------|-----------|---------|
| NEW → IN_PROGRESS | assignee | Все blocked_by в DONE |
| NEW → IN_PROGRESS | любой (claim) | Нет assignee, все blocked_by в DONE, задача public |
| NEW → CANCELLED | creator | — |
| IN_PROGRESS → DONE | assignee | — |
| IN_PROGRESS → BLOCKED | assignee | Вручную блокирует свою задачу |
| IN_PROGRESS → BLOCKED | любой (эскалация) | Чужая задача |
| IN_PROGRESS → NEW | assignee | Отказ от задачи, assignee снимается |
| IN_PROGRESS → CANCELLED | creator или assignee | — |
| BLOCKED → IN_PROGRESS | assignee | Все blocked_by в DONE |
| BLOCKED → NEW | creator или assignee | assignee снимается |
| BLOCKED → CANCELLED | creator | — |
| STUCK → IN_PROGRESS | любой (перехват) | assignee меняется на перехватчика |
| STUCK → IN_PROGRESS | текущий assignee | Возвращает свою задачу в работу |
| STUCK → NEW | creator или system | assignee снимается |
| STUCK → CANCELLED | creator | — |
| * → STUCK | СИСТЕМА | Автоматически при истечении status_deadline_at |

Каждый переход требует обязательный комментарий.

---

## 3. Побочные эффекты переходов

### 3.1 Пересчёт дедлайна

При смене статуса на NEW, IN_PROGRESS или BLOCKED:
```
status_deadline_at = now() + workspace.status_deadlines[новый_статус]
```

При переходе в DONE, CANCELLED, STUCK:
```
status_deadline_at = NULL
```

### 3.2 Снятие assignee

При переходах → NEW (из IN_PROGRESS, BLOCKED, STUCK):
- `assignee_id` устанавливается в NULL
- Задача возвращается в общий пул

### 3.3 Создание событий

Каждый переход создаёт TaskEvent с типом, соответствующим действию:
- Обычная смена статуса → `status_changed`
- Claim → `claimed`
- Эскалация → `escalated`
- Перехват → `taken_over`
- Истечение дедлайна → `deadline_expired`

---

## 4. Бизнес-операции

### 4.1 Claim (проактивный захват)

Агент берёт свободную задачу. Предусловия:
1. Задача в статусе NEW
2. assignee_id is NULL
3. Задача public
4. Все blocked_by в статусе DONE

Результат: assignee = агент, статус = IN_PROGRESS.

Race condition: если два агента одновременно пытаются взять — побеждает первый, второй получает ошибку `TASK_ALREADY_CLAIMED`.

### 4.2 Эскалация

Агент блокирует чужую задачу. Предусловия:
1. Задача в статусе IN_PROGRESS
2. Текущий агент ≠ assignee (нельзя эскалировать свою)

Результат: статус = BLOCKED.

### 4.3 Перехват (takeover)

Агент забирает зависшую задачу. Предусловия:
1. Задача в статусе STUCK
2. Текущий агент ≠ текущий assignee

Результат: assignee = агент, статус = IN_PROGRESS.

Если assignee хочет вернуть свою STUCK-задачу — он использует обычную смену статуса.

### 4.4 Зависимости (blocked_by)

1. Задаются при создании задачи
2. Содержат список ID задач, которые должны быть DONE
3. Задача с неразрешёнными блокерами не может перейти в IN_PROGRESS
4. Циклические зависимости запрещены (проверка при создании — DFS)
5. blocked_by нельзя менять после создания (альфа-версия)

---

## 5. Фоновый процесс — Deadline Checker

- Запускается каждую минуту (интервал настраивается)
- Находит все задачи, где `status_deadline_at < now()` и статус IN (NEW, IN_PROGRESS, BLOCKED)
- Переводит каждую в STUCK
- Создаёт системное событие `deadline_expired` с комментарием: `"Status deadline expired. Was in {old_status} for {duration} minutes."`
- Каждая задача обрабатывается отдельно — ошибка одной не влияет на остальные
- Идемпотентность: повторный запуск не создаёт дублей (задача уже в STUCK → пропускается)
