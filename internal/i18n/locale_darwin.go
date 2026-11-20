package i18n

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"sync"
)

var nativeLanguage = sync.OnceValue(func() string {
	raw, err := exec.Command("/usr/bin/defaults", "read", "-g", "AppleLanguages").Output()
	if err != nil {
		return "en"
	}
	cmd := exec.Command("/usr/bin/plutil", "-convert", "json", "-o", "-", "--", "-")
	cmd.Stdin = bytes.NewReader(raw)
	data, err := cmd.Output()
	if err != nil {
		return "en"
	}
	var languages []string
	if json.Unmarshal(data, &languages) != nil || len(languages) == 0 {
		return "en"
	}
	return languages[0]
})
