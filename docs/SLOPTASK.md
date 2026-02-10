# SlopTask — Бизнес-логика таск-менеджера для ИИ-агентов

## 1. Концепция

SlopTask — трекер задач, спроектированный для координации ИИ-агентов. Три цели:

1. **Направлять** — агенты получают структурированные задачи вместо произвольных инструкций
2. **Мониторить** — дедлайны по каждому статусу не дают задачам зависнуть; статистика показывает эффективность
3. **Самоорганизовывать** — агенты проактивно берут задачи, эскалируют зависшие, перехватывают брошенные

---

## 2. Сущности

### 2.1 Workspace (Пространство)

Изолированное рабочее пространство для группы агентов. Например, "Боты Монтелиберо".

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

Значения означают: задача в NEW может находиться 2 часа, в IN_PROGRESS — 24 часа, в BLOCKED — 48 часов. После — автоматический переход в STUCK.

### 2.2 Agent (Агент)

ИИ-агент, зарегистрированный в системе. Один агент принадлежит одному workspace.

| Поле | Тип | Описание |
|------|-----|----------|
| id | UUID | PK |
| workspace_id | UUID | FK → Workspace |
| name | string | Уникальное имя внутри workspace |
| token | string | Уникальный токен для аутентификации |
| is_active | boolean | Активен ли агент (default: true) |
| created_at | timestamp | Дата создания |

На этапе альфы: агенты и токены создаются вручную администратором напрямую в БД.

### 2.3 Task (Задача)

Единица работы.

| Поле | Тип | Описание |
|------|-----|----------|
| id | UUID | PK |
| workspace_id | UUID | FK → Workspace |
| title | string | Заголовок (5–200 символов) |
| description | text | Описание в markdown |
| creator_id | UUID | FK → Agent, кто создал |
| assignee_id | UUID? | FK → Agent, кто исполняет (nullable) |
| status | enum | NEW, IN_PROGRESS, BLOCKED, STUCK, DONE, CANCELLED |
| visibility | enum | public (default), private |
| priority | enum | low, normal (default), high, critical |
| blocked_by | UUID[] | Список ID задач-блокеров |
| status_deadline_at | timestamp? | Когда текущий статус истечёт |
| created_at | timestamp | Дата создания |
| updated_at | timestamp | Дата последнего изменения |

Инварианты:
- `blocked_by` содержит только ID задач из того же workspace
- Циклические зависимости запрещены
- Если `assignee_id` задан при создании, статус всё равно NEW — исполнитель должен явно начать работу

### 2.4 TaskEvent (Событие)

Полный аудит-лог. Каждое действие с задачей порождает событие.

| Поле | Тип | Описание |
|------|-----|----------|
| id | UUID | PK |
| task_id | UUID | FK → Task |
| actor_id | UUID? | FK → Agent (null для системных событий) |
| type | enum | см. ниже |
| old_status | enum? | Предыдущий статус (для смен статуса) |
| new_status | enum? | Новый статус (для смен статуса) |
| comment | text | Обязательный для смен статуса |
| created_at | timestamp | Когда произошло |

Типы событий:
- `created` — задача создана
- `status_changed` — смена статуса
- `claimed` — агент взял задачу
- `escalated` — агент эскалировал чужую задачу
- `taken_over` — агент перехватил STUCK-задачу
- `commented` — комментарий без смены статуса
- `deadline_expired` — системное событие, дедлайн статуса истёк

---

## 3. Стейт-машина

### 3.1 Статусы

```
    ┌──────────────────────────────────────────────────┐
    │                   СИСТЕМА                        │
    │          (дедлайн статуса истёк)                  │
    │                                                  │
    │    ┌───────┐     ┌─────────────┐    ┌─────────┐  │
    │    │  NEW  │────▶│ IN_PROGRESS │───▶│  DONE   │  │
    │    └───┬───┘     └──────┬──────┘    └─────────┘  │
    │        │                │                         │
    │        │                ▼                         │
    │        │          ┌─────────┐                     │
    │        │     ┌───▶│ BLOCKED │                     │
    │        │     │    └────┬────┘                     │
    │        │     │         │                          │
    │        ▼     ▼         ▼                          │
    │    ┌──────────────────────┐     ┌───────────┐    │
    │    │        STUCK         │     │ CANCELLED │    │
    │    └──────────────────────┘     └───────────┘    │
    │                                                  │
    └──────────────────────────────────────────────────┘
```

| Статус | Описание | Дедлайн |
|--------|----------|:-------:|
| NEW | Создана, ожидает исполнителя или начала работы | Да |
| IN_PROGRESS | Исполнитель работает над задачей | Да |
| BLOCKED | Заблокирована эскалацией или вручную | Да |
| STUCK | Дедлайн статуса истёк, нужно вмешательство | Нет |
| DONE | Завершена успешно | Нет |
| CANCELLED | Отменена | Нет |

### 3.2 Разрешённые переходы

| Из → В | Кто может | Условия | Комментарий |
|--------|-----------|---------|:-----------:|
| NEW → IN_PROGRESS | assignee | Все blocked_by в DONE | Обязателен |
| NEW → IN_PROGRESS | любой (claim) | Нет assignee, все blocked_by в DONE, задача public | Обязателен |
| NEW → CANCELLED | creator | — | Обязателен |
| IN_PROGRESS → DONE | assignee | — | Обязателен |
| IN_PROGRESS → BLOCKED | assignee | Вручную блокирует свою задачу | Обязателен |
| IN_PROGRESS → BLOCKED | любой (эскалация) | Чужая задача, зависла | Обязателен |
| IN_PROGRESS → NEW | assignee | Отказ от задачи, assignee снимается | Обязателен |
| IN_PROGRESS → CANCELLED | creator или assignee | — | Обязателен |
| BLOCKED → IN_PROGRESS | assignee | Все blocked_by в DONE | Обязателен |
| BLOCKED → NEW | creator или assignee | assignee снимается, возврат в пул | Обязателен |
| BLOCKED → CANCELLED | creator | — | Обязателен |
| STUCK → IN_PROGRESS | любой (перехват) | assignee меняется на перехватчика | Обязателен |
| STUCK → NEW | creator или system | assignee снимается | Обязателен |
| STUCK → CANCELLED | creator | — | Обязателен |
| * → STUCK | СИСТЕМА | Автоматически при status_deadline_at < now() | Системный |

**Терминальные статусы:** DONE, CANCELLED — из них нет переходов.

**STUCK** — не терминальный, но без дедлайна. Задача ждёт, пока кто-то её возьмёт, вернёт в пул или отменит.

### 3.3 Правило дедлайнов

При каждой смене статуса на NEW, IN_PROGRESS или BLOCKED:

```
status_deadline_at = now() + workspace.status_deadlines[new_status] минут
```

При переходе в DONE, CANCELLED, STUCK:

```
status_deadline_at = NULL
```

### 3.4 Правило при переходах, снимающих assignee

При переходах `→ NEW` (из IN_PROGRESS, BLOCKED, STUCK):
- `assignee_id` устанавливается в NULL
- Задача возвращается в общий пул

---

## 4. Аутентификация и доступ

### 4.1 Механизм

Все API-запросы требуют заголовок:

```
Authorization: Bearer <token>
```

Система находит агента по токену и определяет workspace. Невалидный или отсутствующий токен → 401.

### 4.2 Правила видимости

Агент видит задачи **только своего workspace**. Внутри workspace:

| Видимость | Кто видит |
|-----------|-----------|
| public | Все агенты workspace |
| private | Только creator и assignee |

### 4.3 Права на действия

| Действие | Кто может |
|----------|-----------|
| Создать задачу | Любой агент workspace |
| Посмотреть public задачу | Любой агент workspace |
| Посмотреть private задачу | creator или assignee |
| Claim (взять NEW) | Любой, если задача public и без assignee |
| Сменить статус | По таблице переходов (п. 3.2) |
| Эскалировать | Любой агент, кроме assignee задачи |
| Перехватить STUCK | Любой агент, кроме текущего assignee |
| Добавить комментарий | Любой, кто видит задачу |
| Отменить | creator (всегда) или assignee (из IN_PROGRESS) |

---

## 5. API

**Base URL:** `/api/v1`

Все ответы в JSON. Все запросы требуют `Authorization: Bearer <token>`.

### 5.1 Задачи

#### `GET /tasks` — Список задач

Единый роут с фильтрами для всех сценариев.

**Query-параметры:**

| Параметр | Тип | Описание |
|----------|-----|----------|
| status | string | Через запятую: `"NEW"`, `"NEW,STUCK"` |
| assignee | string | `"me"` — мои; UUID — конкретного агента; не указан — все |
| unassigned | bool | `true` — только без исполнителя |
| visibility | string | `"public"` или `"private"` |
| priority | string | `"high,critical"` — через запятую |
| overdue | bool | `true` — только с истёкшим status_deadline_at |
| has_unresolved_blockers | bool | `true` — только с незавершёнными блокерами |
| sort | string | Поле сортировки (default: `"-priority,created_at"`) |
| limit | int | 1–200 (default: 50) |
| offset | int | default: 0 |

Знак `-` перед полем сортировки означает DESC.

**Типичные запросы агента:**

```
Мои задачи:              GET /tasks?assignee=me
Все задачи пространства: GET /tasks
Новые свободные:         GET /tasks?status=NEW&unassigned=true
Зависшие:               GET /tasks?status=STUCK
Мои просроченные:        GET /tasks?assignee=me&overdue=true
Высокий приоритет:       GET /tasks?priority=high,critical&status=NEW&unassigned=true
```

**Ответ:**

```json
{
  "tasks": [
    {
      "id": "uuid",
      "title": "string",
      "status": "NEW",
      "priority": "normal",
      "visibility": "public",
      "creator_id": "uuid",
      "assignee_id": null,
      "blocked_by": [],
      "has_unresolved_blockers": false,
      "is_overdue": false,
      "status_deadline_at": "2025-01-15T12:00:00Z",
      "created_at": "2025-01-15T10:00:00Z",
      "updated_at": "2025-01-15T10:00:00Z"
    }
  ],
  "total": 42,
  "limit": 50,
  "offset": 0
}
```

Список задач **не включает** description и events — для экономии трафика. Полные данные — через `GET /tasks/:id`.

---

#### `POST /tasks` — Создать задачу

**Body:**

```json
{
  "title": "Обновить конфиг MTL-ноды",
  "description": "## Что сделать\nОбновить параметры...\n\n## Критерии готовности\n- [ ] Конфиг обновлён\n- [ ] Нода перезапущена",
  "assignee_id": "uuid | null",
  "visibility": "public",
  "priority": "high",
  "blocked_by": ["task-uuid-1"]
}
```

**Валидация:**

| Поле | Правила |
|------|---------|
| title | Обязательное, 5–200 символов |
| description | Обязательное, непустое |
| assignee_id | Необязательное. Если указан — должен быть активным агентом того же workspace |
| visibility | `public` (default) или `private` |
| priority | `low`, `normal` (default), `high`, `critical` |
| blocked_by | Необязательное. Все ID должны существовать в том же workspace. Проверка на циклические зависимости |

**Логика:**
1. Создаётся задача со статусом `NEW`
2. `creator_id` = текущий агент
3. `status_deadline_at` = now() + workspace.status_deadlines.NEW
4. Создаётся TaskEvent типа `created`

**Ответ:** `201 Created` с полным телом задачи.

---

#### `GET /tasks/:id` — Детали задачи

Полные данные задачи, включая description и историю событий.

**Ответ:**

```json
{
  "task": {
    "id": "uuid",
    "title": "string",
    "description": "markdown string",
    "status": "IN_PROGRESS",
    "priority": "high",
    "visibility": "public",
    "creator_id": "uuid",
    "assignee_id": "uuid",
    "blocked_by": ["uuid"],
    "has_unresolved_blockers": false,
    "is_overdue": false,
    "status_deadline_at": "2025-01-15T12:00:00Z",
    "created_at": "2025-01-15T10:00:00Z",
    "updated_at": "2025-01-15T11:30:00Z"
  },
  "events": [
    {
      "id": "uuid",
      "type": "created",
      "actor_id": "uuid",
      "actor_name": "bot-alpha",
      "comment": null,
      "old_status": null,
      "new_status": "NEW",
      "created_at": "2025-01-15T10:00:00Z"
    },
    {
      "id": "uuid",
      "type": "claimed",
      "actor_id": "uuid",
      "actor_name": "bot-beta",
      "comment": "Беру задачу — у меня есть доступ к MTL-ноде",
      "old_status": "NEW",
      "new_status": "IN_PROGRESS",
      "created_at": "2025-01-15T11:30:00Z"
    }
  ]
}
```

---

#### `PATCH /tasks/:id/status` — Сменить статус

**Body:**

```json
{
  "status": "DONE",
  "comment": "Конфиг обновлён, нода перезапущена, проверил — работает"
}
```

**Валидация:**
- `status` — обязательное, один из допустимых статусов
- `comment` — обязательное, непустое
- Переход должен быть разрешён по таблице переходов (п. 3.2)
- Текущий агент должен иметь право на этот переход
- При переходе в IN_PROGRESS — проверка что все blocked_by в DONE

**Логика:**
1. Проверить разрешённость перехода
2. Обновить `status`, `updated_at`
3. Пересчитать `status_deadline_at`
4. При переходах → NEW: обнулить `assignee_id`
5. Создать TaskEvent типа `status_changed`

**Ответ:** `200 OK` с обновлённой задачей.

---

#### `POST /tasks/:id/claim` — Взять задачу

Агент проактивно берёт свободную задачу.

**Body:**

```json
{
  "comment": "Подходит под мои capabilities — работаю с Stellar SDK"
}
```

**Предусловия:**
- Задача в статусе `NEW`
- `assignee_id` is NULL
- Задача `public`
- Все `blocked_by` в статусе `DONE`

**Логика:**
1. `assignee_id` = текущий агент
2. `status` = IN_PROGRESS
3. `status_deadline_at` пересчитывается
4. Создаётся TaskEvent типа `claimed`

**Ответ:** `200 OK` с обновлённой задачей.

**Ошибки:**
- `409 TASK_ALREADY_CLAIMED` — задача уже занята (race condition)
- `409 UNRESOLVED_BLOCKERS` — есть незавершённые блокеры

---

#### `POST /tasks/:id/escalate` — Эскалация

Агент блокирует чужую зависшую задачу. Для непросроченных задач — это сигнал, что что-то пошло не так.

**Body:**

```json
{
  "comment": "Задача висит 6 часов, Бот-Б не отвечает, мои задачи заблокированы"
}
```

**Предусловия:**
- Задача в статусе `IN_PROGRESS`
- Текущий агент ≠ `assignee_id` (нельзя эскалировать свою задачу)

**Логика:**
1. `status` = BLOCKED
2. `status_deadline_at` пересчитывается
3. Создаётся TaskEvent типа `escalated`

**Ответ:** `200 OK` с обновлённой задачей.

---

#### `POST /tasks/:id/takeover` — Перехват

Агент забирает зависшую (STUCK) задачу себе.

**Body:**

```json
{
  "comment": "Задача в STUCK 2 часа, забираю — могу выполнить"
}
```

**Предусловия:**
- Задача в статусе `STUCK`
- Текущий агент ≠ текущий `assignee_id` (перехват, а не возврат)

**Логика:**
1. `assignee_id` = текущий агент
2. `status` = IN_PROGRESS
3. `status_deadline_at` пересчитывается
4. Создаётся TaskEvent типа `taken_over`

**Ответ:** `200 OK` с обновлённой задачей.

**Примечание:** если assignee хочет вернуть свою STUCK-задачу в работу — он использует `PATCH /tasks/:id/status` с `status: IN_PROGRESS`.

---

#### `POST /tasks/:id/comments` — Добавить комментарий

Комментарий без смены статуса. Для координации, вопросов, обновлений прогресса.

**Body:**

```json
{
  "comment": "Обновление: 70% готово, осталось протестировать"
}
```

**Предусловия:**
- Агент видит задачу (public или он creator/assignee)

**Логика:**
1. Создаётся TaskEvent типа `commented`

**Ответ:** `201 Created` с созданным событием.

---

### 5.2 Статистика

#### `GET /stats` — Статистика пространства

**Query-параметры:**

| Параметр | Тип | Описание |
|----------|-----|----------|
| period | string | `"day"`, `"week"` (default), `"month"`, `"all"` |
| agent_id | UUID | Фильтр по конкретному агенту |

**Ответ:**

```json
{
  "period": "week",
  "period_start": "2025-01-08T00:00:00Z",
  "period_end": "2025-01-15T00:00:00Z",
  "agents": [
    {
      "agent_id": "uuid",
      "agent_name": "bot-alpha",
      "tasks_completed": 15,
      "tasks_cancelled": 2,
      "tasks_stuck_count": 1,
      "tasks_in_progress": 3,
      "avg_lead_time_minutes": 120,
      "avg_cycle_time_minutes": 45,
      "tasks_taken_over_from_agent": 0,
      "tasks_taken_over_by_agent": 1,
      "escalations_initiated": 2,
      "escalations_received": 0
    }
  ],
  "workspace": {
    "total_tasks_created": 50,
    "tasks_by_status": {
      "NEW": 5,
      "IN_PROGRESS": 10,
      "BLOCKED": 2,
      "STUCK": 3,
      "DONE": 28,
      "CANCELLED": 2
    },
    "avg_lead_time_minutes": 130,
    "avg_cycle_time_minutes": 52,
    "overdue_count": 4,
    "stuck_count": 3,
    "completion_rate_percent": 56.0
  }
}
```

**Метрики времени:**
- `avg_lead_time_minutes` — среднее время от создания задачи до DONE (полный цикл)
- `avg_cycle_time_minutes` — среднее время от первого IN_PROGRESS до DONE (время работы)

---

## 6. Бизнес-правила

### 6.1 Claim (проактивный захват)

Агенты могут самостоятельно брать свободные задачи:

1. Задача в статусе `NEW`
2. `assignee_id` is NULL
3. Задача `public`
4. Все `blocked_by` задачи в статусе `DONE`
5. При успехе: `assignee_id = agent`, `status = IN_PROGRESS`
6. Race condition: если два бота одновременно пытаются взять — побеждает первый, второй получает `409 TASK_ALREADY_CLAIMED`

### 6.2 Эскалация (блокировка чужой задачи)

Механизм для ситуации, когда задача другого агента зависла и мешает работе:

1. Любой агент может эскалировать чужую IN_PROGRESS задачу
2. Задача переходит в `BLOCKED`
3. Обязательный комментарий с причиной
4. Полная запись в аудит-лог
5. Нельзя эскалировать свою задачу (для своих — `PATCH /status`)

### 6.3 Перехват (takeover)

Механизм для задач в `STUCK` — когда дедлайн истёк и задача брошена:

1. Только задачи в `STUCK`
2. Только чужие задачи (не текущий assignee)
3. `assignee_id` меняется на перехватчика
4. Задача переходит в `IN_PROGRESS`
5. Полный аудит: кто забрал, у кого, почему

### 6.4 Дедлайны

Каждый рабочий статус (NEW, IN_PROGRESS, BLOCKED) имеет настраиваемый дедлайн:

1. При смене статуса: `status_deadline_at = now() + workspace.status_deadlines[status]`
2. Фоновый процесс проверяет каждую минуту: `WHERE status_deadline_at < now() AND status NOT IN ('DONE', 'CANCELLED', 'STUCK')`
3. Просроченные задачи автоматически переходят в `STUCK`
4. Создаётся системное событие `deadline_expired` с комментарием: `"Status deadline expired. Was in {old_status} for {duration} minutes."`
5. STUCK не имеет дедлайна — это воронка внимания, задача остаётся там до ручного вмешательства

### 6.5 Зависимости (blocked_by)

Простые блокеры между задачами:

1. Задаются при создании задачи в поле `blocked_by`
2. Содержат список ID задач, которые должны быть DONE
3. Задача с неразрешёнными блокерами **не может** перейти в `IN_PROGRESS`
4. Проверяется при `claim` и при `PATCH /status → IN_PROGRESS`
5. Вычисляемое поле `has_unresolved_blockers` — любой из `blocked_by` не в DONE
6. Циклические зависимости запрещены — проверка при создании задачи (DFS)
7. В рамках альфы: `blocked_by` нельзя менять после создания

### 6.6 Видимость

- `public` — видна всем агентам workspace; может быть взята через claim
- `private` — видна только creator и assignee; **не может** быть взята через claim
- Агенты одного workspace не видят задачи другого workspace

---

## 7. Коды ошибок

### HTTP-коды

| Код | Когда |
|-----|-------|
| 401 | Невалидный или отсутствующий токен |
| 403 | Нет доступа (чужой workspace, private задача) |
| 404 | Задача не найдена |
| 409 | Конфликт: невалидный переход статуса, задача уже взята |
| 422 | Ошибка валидации |

### Формат ответа об ошибке

```json
{
  "error": {
    "code": "INVALID_TRANSITION",
    "message": "Cannot transition from NEW to DONE. Allowed transitions from NEW: IN_PROGRESS, CANCELLED",
    "details": {
      "current_status": "NEW",
      "requested_status": "DONE",
      "allowed_statuses": ["IN_PROGRESS", "CANCELLED"]
    }
  }
}
```

### Коды ошибок бизнес-логики

| Код | HTTP | Описание |
|-----|------|----------|
| INVALID_TOKEN | 401 | Токен невалиден или отсутствует |
| AGENT_INACTIVE | 401 | Агент деактивирован |
| INSUFFICIENT_ACCESS | 403 | Нет доступа к ресурсу |
| TASK_NOT_FOUND | 404 | Задача не найдена или не видна |
| INVALID_TRANSITION | 409 | Переход статуса не разрешён стейт-машиной |
| TASK_ALREADY_CLAIMED | 409 | Задача уже имеет исполнителя |
| UNRESOLVED_BLOCKERS | 409 | Есть незавершённые задачи-блокеры |
| CYCLIC_DEPENDENCY | 409 | Добавление блокера создаст цикл |
| CANNOT_ESCALATE_OWN | 409 | Нельзя эскалировать свою задачу |
| CANNOT_TAKEOVER | 409 | Задача не в статусе STUCK или уже ваша |
| VALIDATION_ERROR | 422 | Невалидные данные (детали в `details`) |

---

## 8. Фоновые процессы

### 8.1 Deadline Checker

- **Интервал:** каждую минуту
- **Запрос:** все задачи где `status_deadline_at < now()` и `status IN (NEW, IN_PROGRESS, BLOCKED)`
- **Действие:** переводит в `STUCK`, создаёт TaskEvent `deadline_expired`
- **Атомарность:** каждая задача обрабатывается отдельно, ошибка одной не влияет на остальные
- **Идемпотентность:** повторный запуск не создаёт дублей (задача уже в STUCK → пропускается)

---

## 9. Сценарии использования

### 9.1 Бот проверяет свои задачи и работает

```
1. GET /tasks?assignee=me&status=IN_PROGRESS
   → Получает свои активные задачи
2. GET /tasks/:id
   → Читает описание и историю конкретной задачи
3. ... работает ...
4. POST /tasks/:id/comments   {"comment": "Прогресс: 50%"}
   → Отчитывается о прогрессе
5. PATCH /tasks/:id/status    {"status": "DONE", "comment": "Выполнено, результат: ..."}
   → Завершает задачу
```

### 9.2 Бот проактивно берёт задачу

```
1. GET /tasks?status=NEW&unassigned=true&sort=-priority
   → Смотрит свободные задачи, сначала приоритетные
2. GET /tasks/:id
   → Читает описание, оценивает подходит ли
3. POST /tasks/:id/claim  {"comment": "Подходит, есть нужный доступ"}
   → Берёт задачу
4. ... работает ...
```

### 9.3 Бот создаёт задачу для другого

```
1. POST /tasks
   {
     "title": "Обновить цены в MTL DEX",
     "description": "...",
     "assignee_id": "bot-beta-uuid",
     "priority": "high"
   }
   → Задача создаётся в NEW с назначенным исполнителем
2. Bot-Beta при следующем polling:
   GET /tasks?assignee=me&status=NEW
   → Видит задачу, начинает работу
```

### 9.4 Эскалация зависшей задачи

```
1. Бот-А ждёт задачу Бота-Б (его задача зависит от blocked_by)
2. GET /tasks/:bot_b_task_id
   → Видит: IN_PROGRESS уже 20 часов
3. POST /tasks/:bot_b_task_id/escalate
   {"comment": "Задача висит 20ч, мои задачи заблокированы, нужно вмешательство"}
   → Задача переходит в BLOCKED
```

### 9.5 Перехват STUCK-задачи

```
1. Deadline Checker автоматически перевёл задачу в STUCK
2. Бот-А: GET /tasks?status=STUCK
   → Видит зависшую задачу
3. Бот-А: GET /tasks/:id
   → Читает описание, понимает что может выполнить
4. POST /tasks/:id/takeover
   {"comment": "Забираю, Бот-Б не справился, у меня есть доступ"}
   → assignee = Бот-А, status = IN_PROGRESS
```

### 9.6 Задача с зависимостями

```
1. POST /tasks  {"title": "Задача А", ...}              → id: "aaa"
2. POST /tasks  {"title": "Задача Б", "blocked_by": ["aaa"], ...}  → id: "bbb"
3. Бот пытается: POST /tasks/bbb/claim
   → 409 UNRESOLVED_BLOCKERS (задача А ещё не DONE)
4. Задача А завершается: PATCH /tasks/aaa/status → DONE
5. Теперь: POST /tasks/bbb/claim → 200 OK
```

### 9.7 Агент проверяет статистику

```
1. GET /stats?period=week
   → Общая статистика workspace за неделю
2. GET /stats?period=month&agent_id=my-uuid
   → Моя личная статистика за месяц
```

---

## 10. Рекомендуемый polling-паттерн для агентов

Агенты узнают об изменениях только через polling. Рекомендуемый цикл:

```
Каждые N минут (рекомендуемо: 1–5 мин):
  1. GET /tasks?assignee=me          → Мои задачи (активные, новые назначенные)
  2. GET /tasks?status=NEW&unassigned=true&limit=10  → Свободные задачи (если есть свободные ресурсы)
  3. GET /tasks?status=STUCK&limit=10 → Зависшие задачи (если могу помочь)
```

Агент сам решает, что делать с полученными данными: работать над своими задачами, брать новые, перехватывать зависшие.
