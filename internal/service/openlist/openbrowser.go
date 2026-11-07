package openlist

import "strings"

// tokenPageURL 返回 OpenList 生成访问令牌的页面地址。
//
// 令牌在后台「设置 → 其他」页面最下方生成，没有独立路由。
func tokenPageURL(base string) string {
	return normalizeURL(base) + "/@manage/settings/other"
}

// normalizeURL 补协议、去尾部斜杠。空串原样返回。
func normalizeURL(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimRight(raw, "/")
	if raw == "" {
		return ""
	}
	lowered := strings.ToLower(raw)
	if !strings.HasPrefix(lowered, "http://") && !strings.HasPrefix(lowered, "https://") {
		raw = "http://" + raw
	}
	return raw
}
