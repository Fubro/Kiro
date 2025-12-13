package utils

import (
	"encoding/json"
	"fmt"
	"time"
)

// LogField 日志字段
type LogField struct {
	Key   string
	Value any
}

// Log 统一日志输出
func Log(msg string, fields ...LogField) {
	entry := map[string]any{
		"time":    time.Now().Format("15:04:05"),
		"message": msg,
	}
	for _, f := range fields {
		entry[f.Key] = f.Value
	}
	data, _ := json.Marshal(entry)
	fmt.Println(string(data))
}

// 字段构造函数
func LogString(key, val string) LogField { return LogField{Key: key, Value: val} }
func LogInt(key string, val int) LogField { return LogField{Key: key, Value: val} }
func LogBool(key string, val bool) LogField { return LogField{Key: key, Value: val} }
func LogAny(key string, val any) LogField  { return LogField{Key: key, Value: val} }

func LogErr(err error) LogField {
	if err == nil {
		return LogField{Key: "error", Value: nil}
	}
	return LogField{Key: "error", Value: err.Error()}
}
