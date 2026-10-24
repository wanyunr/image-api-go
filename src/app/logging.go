package app

import (
	"encoding/json"
	"log"
	"regexp"
	"strings"
	"time"
)

// tokenQueryRe 匹配 query 串中的 token 参数（大小写不敏感，含 & 连接符场景），用于日志脱敏
var tokenQueryRe = regexp.MustCompile(`(?i)(^|&)token=[^&]*`)

// logStructured 输出 JSON 格式的结构化日志。
func logStructured(event string, fields map[string]any) {
	m := make(map[string]any, len(fields)+2)
	m["event"] = event
	m["ts"] = time.Now().UTC().Format(time.RFC3339)
	for k, v := range fields {
		m[k] = v
	}
	b, _ := json.Marshal(m)
	log.Println(string(b))
}

// sanitizeQuery 对 query 串做脱敏：将 token 参数值替换为 ***，避免敏感信息进入日志。
// 非法参数（如未编码的 % 序列）原样保留，不做二次解析，防止误改。
func sanitizeQuery(raw string) string {
	if raw == "" || !strings.Contains(strings.ToLower(raw), "token") {
		return raw
	}
	return tokenQueryRe.ReplaceAllString(raw, "${1}token=***")
}
