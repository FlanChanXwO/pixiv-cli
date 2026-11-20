package i18n

import "golang.org/x/sys/windows"

func nativeLanguage() string {
	languages, err := windows.GetUserPreferredUILanguages(windows.MUI_LANGUAGE_NAME)
	if err != nil || len(languages) == 0 {
		return "en"
	}
	return languages[0]
}
