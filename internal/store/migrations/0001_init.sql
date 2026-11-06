-- 0001_init: 任务主表与迁移记录表.

CREATE TABLE IF NOT EXISTS tasks (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    sort_order     INTEGER NOT NULL DEFAULT 0,
    name           TEXT    NOT NULL,
    program        TEXT    NOT NULL,
    args           TEXT    NOT NULL DEFAULT '[]',
    stdin          TEXT,
    stdout         TEXT,
    stderr         TEXT,
    working_dir    TEXT,
    env_vars       TEXT    NOT NULL DEFAULT '{}',
    trigger_tag    TEXT    NOT NULL,
    trigger_value  TEXT,
    enabled        INTEGER NOT NULL DEFAULT 1,
    no_console     INTEGER NOT NULL DEFAULT 0,
    last_exit_code INTEGER,
    last_run_at    TEXT,
    created_at     TEXT    NOT NULL,
    updated_at     TEXT    NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_tasks_sort_order ON tasks(sort_order);
CREATE INDEX IF NOT EXISTS idx_tasks_enabled ON tasks(enabled);
