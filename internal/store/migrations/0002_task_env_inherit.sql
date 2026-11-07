-- 0002_task_env_inherit: 任务可分别选择是否继承 shell 注入与全局注入的环境变量.
--
-- 两个开关分开控制, 因为它们的来源和用途不同:
--   use_shell_env   继承从登录 shell 读取的环境 (主要是 PATH, 让 homebrew 与 SDK 里的命令可用)
--   use_global_env  继承设置里手工填写的全局变量
--
-- 都默认 1: 从 Finder 启动的应用 PATH 只有系统默认值, 默认继承对绝大多数任务是对的.
-- 需要隔离环境时按需关掉; 任务自己的 env_vars 始终生效且优先级最高.

ALTER TABLE tasks ADD COLUMN use_shell_env INTEGER NOT NULL DEFAULT 1;
ALTER TABLE tasks ADD COLUMN use_global_env INTEGER NOT NULL DEFAULT 1;
