package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	DB "github.com/libost/sticker_go/database"
)

//go:embed *.json
var i18nFiles embed.FS

func GetLocalisedString(key string, languageCode string) string {
	if text, ok := getStringFromLanguageFile(key, languageCode); ok {
		return text
	}

	if languageCode != "en" {
		if text, ok := getStringFromLanguageFile(key, "en"); ok {
			return text
		}
	}

	return key
}

func getStringFromLanguageFile(key string, languageCode string) (string, bool) {
	data, err := i18nFiles.ReadFile(languageCode + ".json")
	if err != nil {
		return "", false
	}

	var translations map[string]any
	if err := json.Unmarshal(data, &translations); err != nil {
		return "", false
	}

	return getNestedString(translations, key)
}

func getNestedString(root map[string]any, key string) (string, bool) {
	if key == "" {
		return "", false
	}

	segments := strings.Split(key, ".")
	var current any = root

	for _, segment := range segments {
		nodeKey, arrayIndex, hasArrayIndex, ok := parseSegment(segment)
		if !ok {
			return "", false
		}

		node, ok := current.(map[string]any)
		if !ok {
			return "", false
		}

		next, exists := node[nodeKey]
		if !exists {
			return "", false
		}
		current = next

		if hasArrayIndex {
			arrayNode, ok := current.([]any)
			if !ok {
				return "", false
			}
			if arrayIndex < 0 || arrayIndex >= len(arrayNode) {
				return "", false
			}
			current = arrayNode[arrayIndex]
		}
	}

	text, ok := current.(string)
	if !ok {
		return "", false
	}

	return text, true
}

func parseSegment(segment string) (key string, index int, hasIndex bool, ok bool) {
	segment = strings.TrimSpace(segment)
	if segment == "" {
		return "", 0, false, false
	}

	openBracket := strings.Index(segment, "[")
	if openBracket == -1 {
		return segment, 0, false, true
	}

	if !strings.HasSuffix(segment, "]") {
		return "", 0, false, false
	}

	key = segment[:openBracket]
	if key == "" {
		return "", 0, false, false
	}

	indexRaw := segment[openBracket+1 : len(segment)-1]
	if indexRaw == "" {
		return "", 0, false, false
	}

	parsedIndex, err := strconv.Atoi(indexRaw)
	if err != nil {
		return "", 0, false, false
	}

	return key, parsedIndex, true, true
}

func LangCodePrefer(uid int64, defaultLang string) string {
	body := map[string]any{
		"type": "get",
	}
	userLang, err := DB.Init("language_code", uid, body)
	if err != nil || !userLang["language_exists"].(bool) {
		return defaultLang
	}
	return userLang["language_code"].(string)
}

// GetAllSupportedLanguages 获取所有支持的语言列表和数量
func GetAllSupportedLanguages() ([]map[string]string, int64, error) {
	files, err := i18nFiles.ReadDir(".")
	if err != nil {
		return nil, 0, err
	}
	var languages []map[string]string
	i := 0
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") {
			continue
		}
		data, err := i18nFiles.ReadFile(file.Name())
		if err != nil {
			return nil, 0, err
		}
		var translations map[string]any
		if err := json.Unmarshal(data, &translations); err != nil {
			return nil, 0, err
		}
		i++
		languageCode, _ := translations["lang_code"].(string)
		languageName, _ := translations["lang_name"].(string)
		languageCodeAlt, _ := translations["lang_code_alt"].(string)

		languages = append(languages, map[string]string{
			fmt.Sprintf("code_%d", i):     languageCode,
			fmt.Sprintf("name_%d", i):     languageName,
			fmt.Sprintf("code_alt_%d", i): languageCodeAlt,
		})
	}
	return languages, int64(i), nil
}
