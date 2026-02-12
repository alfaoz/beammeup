package remote

import (
	"bufio"
	"strconv"
	"strings"
)

type KeyValues map[string]string

func ParseBM(output string) KeyValues {
	kv := KeyValues{}
	s := bufio.NewScanner(strings.NewReader(output))
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if !strings.HasPrefix(line, "BM_") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		kv[parts[0]] = parts[1]
	}
	return kv
}

func (kv KeyValues) Get(key string) string {
	return kv[key]
}

func (kv KeyValues) Bool(key string) bool {
	v := strings.TrimSpace(strings.ToLower(kv[key]))
	return v == "1" || v == "true" || v == "yes"
}

func (kv KeyValues) Int(key string) int {
	v, _ := strconv.Atoi(strings.TrimSpace(kv[key]))
	return v
}
