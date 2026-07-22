package pkg

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
)

func GenerateDiff(oldConfig, newConfig map[string]interface{}) string {
	if oldConfig == nil {
		oldConfig = make(map[string]interface{})
	}
	if newConfig == nil {
		newConfig = make(map[string]interface{})
	}

	var added []string
	var modified []string
	var removed []string

	allKeys := make(map[string]bool)
	for k := range oldConfig {
		allKeys[k] = true
	}
	for k := range newConfig {
		allKeys[k] = true
	}

	for k := range allKeys {
		oldVal, oldExists := oldConfig[k]
		newVal, newExists := newConfig[k]

		if !oldExists && newExists {
			added = append(added, k)
		} else if oldExists && !newExists {
			removed = append(removed, k)
		} else if oldExists && newExists {
			oldJSON, _ := json.Marshal(oldVal)
			newJSON, _ := json.Marshal(newVal)
			if string(oldJSON) != string(newJSON) {
				modified = append(modified, k)
			}
		}
	}

	sort.Strings(added)
	sort.Strings(modified)
	sort.Strings(removed)

	diffStr := ""
	if len(added) > 0 {
		diffStr += fmt.Sprintf("Added: %v; ", added)
	}
	if len(modified) > 0 {
		diffStr += fmt.Sprintf("Modified: %v; ", modified)
	}
	if len(removed) > 0 {
		diffStr += fmt.Sprintf("Removed: %v; ", removed)
	}
	if diffStr == "" {
		diffStr = "No changes"
	}
	return diffStr
}

func HashContent(content map[string]interface{}) string {
	data, _ := json.Marshal(content)
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}
