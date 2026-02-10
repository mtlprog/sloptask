# Часть 4 — API

## Что должно быть готово после этого этапа

Полностью работающий HTTP API со всеми эндпоинтами, аутентификацией, валидацией и правильными кодами ошибок.

---

## 1. Аутентификация

Все запросы требуют заголовок:
```
Authorization: Bearer <token>
```

Система находит агента по токену и определяет workspace. Невалидный или отсутствующий токен → 401.
Деактивированный агент (is_active = false) → 401.

---

## 2. Видимость задач

Агент видит задачи только своего workspace. Внутри workspace:

| Видимость | Кто видит |
|-----------|-----------|
| public | Все агенты workspace |
| private | Только creator и assignee |

---

## 3. Эндпоинты

Base URL: `/api/v1`

### 3.1 `GET /tasks` — Список задач

Фильтры:

| Параметр | Тип | Описание |
|----------|-----|----------|
| status | string | Через запятую: `"NEW"`, `"NEW,STUCK"` |
| assignee | string | `"me"` — мои; UUID — конкретного агента; не указан — все |
| unassigned | bool | `true` — только без исполнителя |
| visibility | string | `"public"` или `"private"` |
| priority | string | `"high,critical"` — через запятую |
| overdue | bool | `true` — только с истёкшим status_deadline_at |
| has_unresolved_blockers | bool | `true` — только с незавершёнными блокерами |
| sort | string | Поле сортировки (default: `"-priority,created_at"`). `-` перед полем = DESC |
| limit | int | 1–200 (default: 50) |
| offset | int | default: 0 |

Ответ содержит краткую информацию о задачах (без description и events). Поля: id, title, status, priority, visibility, creator_id, assignee_id, blocked_by, has_unresolved_blockers, is_overdue, status_deadline_at, created_at, updated_at.

Пагинация: total, limit, offset.

### 3.2 `POST /tasks` — Создать задачу

Поля запроса:

| Поле | Правила |
|------|---------|
| title | Обязательное, 5–200 символов |
| description | Обязательное, непустое |
| assignee_id | Необязательное. Если указан — активный агент того же workspace |
| visibility | `public` (default) или `private` |
| priority | `low`, `normal` (default), `high`, `critical` |
| blocked_by | Необязательное. ID задач того же workspace. Проверка на циклы |

Логика:
1. Статус = NEW
2. creator_id = текущий агент
3. status_deadline_at = now() + workspace.status_deadlines.NEW
4. Создаётся TaskEvent типа `created`

Ответ: `201 Created`.

### 3.3 `GET /tasks/:id` — Детали задачи

Полные данные задачи, включая description и историю событий (events). Каждое событие содержит: id, type, actor_id, actor_name, comment, old_status, new_status, created_at.

### 3.4 `PATCH /tasks/:id/status` — Сменить статус

Поля запроса:
- `status` — обязательное
- `comment` — обязательное, непустое

Валидация:
- Переход разрешён по таблице переходов
- Текущий агент имеет право на переход
- При переходе в IN_PROGRESS — все blocked_by в DONE

Логика:
1. Обновить status, updated_at
2. Пересчитать status_deadline_at
3. При переходах → NEW: обнулить assignee_id
4. Создать TaskEvent

Ответ: `200 OK`.

### 3.5 `POST /tasks/:id/claim` — Взять задачу

Поля запроса:
- `comment` — обязательное

Предусловия: задача NEW, без assignee, public, все blocked_by в DONE.

Ответ: `200 OK`.
Ошибки: `409 TASK_ALREADY_CLAIMED`, `409 UNRESOLVED_BLOCKERS`.

### 3.6 `POST /tasks/:id/escalate` — Эскалация

Поля запроса:
- `comment` — обязательное

Предусловия: задача IN_PROGRESS, текущий агент ≠ assignee.

Ответ: `200 OK`.

### 3.7 `POST /tasks/:id/takeover` — Перехват

Поля запроса:
- `comment` — обязательное

Предусловия: задача STUCK, текущий агент ≠ текущий assignee.

Ответ: `200 OK`.

### 3.8 `POST /tasks/:id/comments` — Комментарий

Поля запроса:
- `comment` — обязательное

Предусловия: агент видит задачу.

Ответ: `201 Created` с созданным событием.

### 3.9 `GET /stats` — Статистика

Фильтры:

| Параметр | Тип | Описание |
|----------|-----|----------|
| period | string | `"day"`, `"week"` (default), `"month"`, `"all"` |
| agent_id | UUID | Фильтр по агенту |

Ответ содержит:

**По каждому агенту:** tasks_completed, tasks_cancelled, tasks_stuck_count, tasks_in_progress, avg_lead_time_minutes, avg_cycle_time_minutes, tasks_taken_over_from_agent, tasks_taken_over_by_agent, escalations_initiated, escalations_received.

**По workspace:** total_tasks_created, tasks_by_status, avg_lead_time_minutes, avg_cycle_time_minutes, overdue_count, stuck_count, completion_rate_percent.

Метрики времени:
- `avg_lead_time_minutes` — среднее от создания до DONE
- `avg_cycle_time_minutes` — среднее от первого IN_PROGRESS до DONE

---

## 4. Коды ошибок

| Код | HTTP | Описание |
|-----|------|----------|
| INVALID_TOKEN | 401 | Токен невалиден или отсутствует |
| AGENT_INACTIVE | 401 | Агент деактивирован |
| INSUFFICIENT_ACCESS | 403 | Нет доступа к ресурсу |
| TASK_NOT_FOUND | 404 | Задача не найдена или не видна |
| INVALID_TRANSITION | 409 | Переход не разрешён стейт-машиной |
| TASK_ALREADY_CLAIMED | 409 | Задача уже имеет исполнителя |
| UNRESOLVED_BLOCKERS | 409 | Незавершённые задачи-блокеры |
| CYCLIC_DEPENDENCY | 409 | Цикл в зависимостях |
| CANNOT_ESCALATE_OWN | 409 | Нельзя эскалировать свою задачу |
| CANNOT_TAKEOVER | 409 | Задача не STUCK или уже ваша |
| VALIDATION_ERROR | 422 | Невалидные данные |

Формат ответа об ошибке:
```json
{
  "error": {
    "code": "СТРОКОВЫЙ_КОД",
    "message": "Человекочитаемое описание",
    "details": {}
  }
}
```
