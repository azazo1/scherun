package store

import (
	"database/sql"
	"strconv"
	"time"

	"github.com/azazo1/scherun/internal/task"
)

// triggerTimeLayout 是 trigger_value 里时间点的存储格式.
const triggerTimeLayout = time.RFC3339

// encodeTriggerValue 把触发参数序列化成单个字符串列.
//
// Routine 存秒数, Instant 存 RFC3339 时间, 其余触发方式没有参数, 存 NULL.
func encodeTriggerValue(trigger task.Trigger) any {
	switch trigger.Kind {
	case task.TriggerRoutine:
		return strconv.Itoa(trigger.IntervalSeconds)
	case task.TriggerInstant:
		if trigger.RunAt == nil {
			return nil
		}
		return trigger.RunAt.Format(triggerTimeLayout)
	default:
		return nil
	}
}

// decodeTrigger 由 tag 与 value 还原触发方式, 参数损坏时退化为手动触发.
func decodeTrigger(tag string, value sql.NullString) task.Trigger {
	kind := task.TriggerKind(tag)
	if !kind.Valid() {
		return task.Trigger{Kind: task.TriggerManual}
	}
	trigger := task.Trigger{Kind: kind}
	if !value.Valid || value.String == "" {
		return trigger
	}
	switch kind {
	case task.TriggerRoutine:
		seconds, err := strconv.Atoi(value.String)
		if err != nil || seconds < 1 {
			return task.Trigger{Kind: task.TriggerManual}
		}
		trigger.IntervalSeconds = seconds
	case task.TriggerInstant:
		parsed, err := time.Parse(triggerTimeLayout, value.String)
		if err != nil {
			return task.Trigger{Kind: task.TriggerManual}
		}
		trigger.RunAt = &parsed
	}
	return trigger
}
