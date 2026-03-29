package cache

import "strings"

func ModelTargetsKey(model string) string {
	return "proxy:model:targets:" + strings.ToLower(model)
}

func ModelRoundRobinKey(model string) string {
	return "proxy:model:rr:" + strings.ToLower(model)
}
