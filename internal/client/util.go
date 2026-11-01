package client

import (
	"net/url"
	"sort"
)

func urlUnquote(s string) (string, error) { return url.PathUnescape(s) }

// queryEscape 与 Python 的 urllib.parse.urlencode 对齐：空格变 +。
func queryEscape(s string) string { return url.QueryEscape(s) }

// sortMounts 按路径长度降序，保证最长前缀优先匹配。
func sortMounts(items []Mount) {
	sort.SliceStable(items, func(i, j int) bool {
		return len(items[i].Path) > len(items[j].Path)
	})
}
