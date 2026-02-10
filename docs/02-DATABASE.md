# Часть 2 — База данных

## Что должно быть готово после этого этапа

Схема БД со всеми таблицами, связями и ограничениями. Seed-данные для разработки.

---

## 1. Workspace (Пространство)

| Поле | Тип | Описание |
|------|-----|----------|
| id | UUID | PK |
| name | string | Название пространства |
| slug | string | Уникальный идентификатор для URL |
| status_deadlines | JSON | Дедлайны по статусам в минутах |
| created_at | timestamp | Дата создания |

Значения `status_deadlines` по умолчанию:

```json
{
  "NEW": 120,
  "IN_PROGRESS": 1440,
  "BLOCKED": 2880
}
```

Ограничения:
- `slug` уникален

---

## 2. Agent (Агент)

| Поле | Тип | Описание |
|------|-----|----------|
| id | UUID | PK |
| workspace_id | UUID | FK → Workspace |
| name | string | Имя агента |
| token | string | Токен для аутентификации |
| is_active | boolean | Активен ли (default: true) |
| created_at | timestamp | Дата создания |

Ограничения:
- `name` уникален внутри workspace
- `token` уникален глобально
- Агенты и токены создаются вручную администратором напрямую в БД (альфа-версия)

---

## 3. Task (Задача)

| Поле | Тип | Описание |
|------|-----|----------|
| id | UUID | PK |
| workspace_id | UUID | FK → Workspace |
| title | string | Заголовок (5–200 символов) |
| description | text | Описание в markdown |
| creator_id | UUID | FK → Agent |
| assignee_id | UUID? | FK → Agent (nullable) |
| status | enum | NEW, IN_PROGRESS, BLOCKED, STUCK, DONE, CANCELLED |
| visibility | enum | public (default), private |
| priority | enum | low, normal (default), high, critical |
| blocked_by | UUID[] | Список ID задач-блокеров |
| status_deadline_at | timestamp? | Когда текущий статус истечёт |
| created_at | timestamp | Дата создания |
| updated_at | timestamp | Дата последнего изменения |

Ограничения:
- `blocked_by` содержит только ID задач из того же workspace
- `title` от 5 до 200 символов
- `status` по умолчанию NEW
- `visibility` по умолчанию public
- `priority` по умолчанию normal

---

## 4. TaskEvent (Событие)

| Поле | Тип | Описание |
|------|-----|----------|
| id | UUID | PK |
| task_id | UUID | FK → Task |
| actor_id | UUID? | FK → Agent (null для системных событий) |
| type | enum | created, status_changed, claimed, escalated, taken_over, commented, deadline_expired |
| old_status | enum? | Предыдущий статус |
| new_status | enum? | Новый статус |
| comment | text | Комментарий (обязателен для смен статуса) |
| created_at | timestamp | Когда произошло |

---

## 5. Seed-данные для разработки

Для удобства разработки и тестирования нужны начальные данные:

- 1 workspace с дефолтными дедлайнами
- 2–3 агента с токенами
