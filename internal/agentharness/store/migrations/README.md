# Agent Harness 数据库迁移

## 概述

本目录包含 Agent Harness 的数据库迁移脚本。

## 迁移文件

- `001_create_agent_tables.sql`：创建 agent_runs、agent_run_attempts、agent_run_steps 表

## 使用方法

### 1. 手动执行迁移

```bash
# 连接到 MySQL 数据库
mysql -u username -p database_name

# 执行迁移脚本
source /path/to/001_create_agent_tables.sql
```

### 2. 使用 GORM AutoMigrate

在开发环境中，可以使用 GORM 的 AutoMigrate 功能：

```go
import (
    "gorm.io/gorm"
    "YoudaoNoteLm/internal/agentharness/store"
)

func AutoMigrate(db *gorm.DB) error {
    return db.AutoMigrate(
        &store.AgentRun{},
        &store.AgentRunAttempt{},
        &store.AgentRunStep{},
    )
}
```

## 表结构说明

### agent_runs

存储 Run 的基本信息，包括状态、版本、执行权等。

### agent_run_attempts

存储 Attempt 的信息，包括 Worker ID、Fencing Token、状态等。

### agent_run_steps

存储 Step 的信息，包括类型、Agent 名称、状态、Artifact 引用等。

## 索引说明

所有表都包含必要的索引以支持常见查询：

- Run ID 索引
- User ID 索引
- State 索引
- Attempt Number 唯一索引
- Kind 和 Agent Name 索引

## 外键约束

所有外键都使用 `RESTRICT` 删除策略，防止意外删除导致数据不一致。

## 注意事项

1. 迁移脚本使用 `IF NOT EXISTS`，可以安全地重复执行
2. 所有时间字段使用 `DATETIME` 类型
3. 字符串字段使用 `VARCHAR` 类型，长度根据业务需求设定
4. 状态字段使用 `VARCHAR(32)` 以支持未来扩展
