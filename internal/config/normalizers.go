// Functions to normalize YAML input before processing into data structure.
package config

import (
	"errors"
	"fmt"
	"strings"

	"github.com/dalibo/ldap2pg/internal/ldap"
	"golang.org/x/exp/maps"
)

type KeyConflict struct {
	Key      string
	Conflict string
}

func (err *KeyConflict) Error() string {
	return fmt.Sprintf("key conflict between %s and %s", err.Key, err.Conflict)
}

type ParseError struct {
	Message string
	Value   interface{}
}

func (err *ParseError) Error() string {
	return err.Message
}

func NormalizeAlias(yaml *map[string]interface{}, key, alias string) (err error) {
	value, hasAlias := (*yaml)[alias]
	if !hasAlias {
		return
	}

	_, hasKey := (*yaml)[key]
	if hasKey {
		return &KeyConflict{
			Key:      key,
			Conflict: alias,
		}
	}

	delete(*yaml, alias)
	(*yaml)[key] = value
	return
}

func NormalizeList(yaml interface{}) (list []interface{}) {
	list, ok := yaml.([]interface{})
	if !ok {
		list = append(list, yaml)
	}
	return
}

func NormalizeStringList(yaml interface{}) (list []string, err error) {
	switch yaml.(type) {
	case nil:
		return
	case string:
		list = append(list, yaml.(string))
	case []interface{}:
		for _, iItem := range yaml.([]interface{}) {
			item, ok := iItem.(string)
			if !ok {
				return nil, errors.New("must be string")
			}
			list = append(list, item)
		}
	case []string:
		list = yaml.([]string)
	default:
		return nil, fmt.Errorf("must be string or list of string, got %v", yaml)
	}
	return
}

func NormalizeConfigRoot(yaml interface{}) (config map[string]interface{}, err error) {
	config, ok := yaml.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("bad type: %T", yaml)
	}

	section, ok := config["postgres"]
	if ok {
		err = NormalizePostgres(section)
		if err != nil {
			return config, fmt.Errorf("postgres: %w", err)
		}
	}

	section, ok = config["sync_map"]
	if !ok {
		return config, errors.New("missing sync_map")
	}
	syncMap, err := NormalizeSyncMap(section)
	if err != nil {
		return config, fmt.Errorf("sync_map: %w", err)
	}
	config["sync_map"] = syncMap
	return
}

func NormalizePostgres(yaml interface{}) error {
	yamlMap, ok := yaml.(map[string]interface{})
	if !ok {
		return fmt.Errorf("bad type: %T, must be a map", yaml)
	}

	err := CheckIsString(yamlMap["fallback_owner"])
	if err != nil {
		return fmt.Errorf("fallback_owner: %w", err)
	}
	return nil
}

func NormalizeSyncMap(yaml interface{}) (syncMap []interface{}, err error) {
	rawItems, ok := yaml.([]interface{})
	if !ok {
		return nil, fmt.Errorf("bad type: %T, must be a list", yaml)
	}
	for i, rawItem := range rawItems {
		var item interface{}
		item, err = NormalizeSyncItem(rawItem)
		if err != nil {
			return syncMap, fmt.Errorf("item %d: %w", i, err)
		}
		syncMap = append(syncMap, item)
	}
	return
}

func NormalizeSyncItem(yaml interface{}) (item map[string]interface{}, err error) {
	item = map[string]interface{}{
		"description": "",
		"ldapsearch":  map[string]interface{}{},
		"roles":       []interface{}{},
		"grants":      []interface{}{},
	}

	yamlMap, ok := yaml.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("bad type: %T, must be a map", yaml)
	}

	err = NormalizeAlias(&yamlMap, "ldapsearch", "ldap")
	if err != nil {
		return
	}
	err = NormalizeAlias(&yamlMap, "roles", "role")
	if err != nil {
		return
	}
	err = NormalizeAlias(&yamlMap, "grants", "grant")
	if err != nil {
		return
	}

	maps.Copy(item, yamlMap)

	err = CheckIsString(item["description"])
	if err != nil {
		return
	}
	search, err := NormalizeLdapSearch(item["ldapsearch"])
	if err != nil {
		return nil, fmt.Errorf("ldapsearch: %w", err)
	}
	item["ldapsearch"] = search

	list := NormalizeList(item["roles"])
	rules := []interface{}{}
	for i, rawRule := range list {
		var rule map[string]interface{}
		rule, err = NormalizeRoleRule(rawRule)
		if err != nil {
			return nil, fmt.Errorf("roles[%d]: %w", i, err)
		}
		for _, rule := range DuplicateRoleRules(rule) {
			rules = append(rules, rule)
		}
	}
	item["roles"] = rules

	err = CheckSpuriousKeys(&item, "description", "ldapsearch", "roles", "grants")
	return
}

func NormalizeLdapSearch(yaml interface{}) (search map[string]interface{}, err error) {
	search, err = NormalizeCommonLdapSearch(yaml)
	if err != nil {
		return
	}
	err = NormalizeAlias(&search, "subsearches", "joins")
	if err != nil {
		return
	}
	err = CheckSpuriousKeys(&search, "base", "filter", "scope", "subsearches", "on_unexpected_dn")
	if err != nil {
		return
	}

	subsearches, ok := search["subsearches"].(map[string]interface{})
	if !ok {
		return
	}
	for attr := range subsearches {
		var subsearch map[string]interface{}
		subsearch, err = NormalizeCommonLdapSearch(subsearches[attr])
		if err != nil {
			return
		}
		subsearches[attr] = subsearch
		err = CheckSpuriousKeys(&subsearch, "filter", "scope")
		if err != nil {
			return
		}
	}
	return
}

func NormalizeCommonLdapSearch(yaml interface{}) (search map[string]interface{}, err error) {
	search = map[string]interface{}{
		"filter": "(objectClass=*)",
		"scope":  "sub",
	}
	yamlMap, ok := yaml.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("bad type: %T", yaml)
	}
	maps.Copy(search, yamlMap)
	search["filter"] = ldap.CleanFilter(search["filter"].(string))
	return
}

func NormalizeRoleRule(yaml interface{}) (rule map[string]interface{}, err error) {
	rule = map[string]interface{}{
		"comment": "Managed by ldap2pg",
		"options": "",
		"parents": []string{},
	}

	switch yaml.(type) {
	case string:
		rule["names"] = []string{yaml.(string)}
	case map[string]interface{}:
		yamlMap := yaml.(map[string]interface{})
		err = NormalizeAlias(&yamlMap, "names", "name")
		if err != nil {
			return
		}
		err = NormalizeAlias(&yamlMap, "parents", "parent")
		if err != nil {
			return
		}

		maps.Copy(rule, yamlMap)

		names, ok := rule["names"]
		if ok {
			rule["names"], err = NormalizeStringList(names)
			if err != nil {
				return
			}
		} else {
			return nil, errors.New("missing name")
		}
		rule["parents"], err = NormalizeStringList(rule["parents"])
		if err != nil {
			return
		}
		rule["options"], err = NormalizeRoleOptions(rule["options"])
		if err != nil {
			return nil, fmt.Errorf("options: %w", err)
		}
	default:
		return nil, fmt.Errorf("bad type: %T", yaml)
	}

	err = CheckSpuriousKeys(&rule, "names", "comment", "parents", "options")
	return
}

// Normalize one rule with a list of names to a list of rules with a single
// name.
func DuplicateRoleRules(yaml map[string]interface{}) (rules []map[string]interface{}) {
	for _, name := range yaml["names"].([]string) {
		rule := make(map[string]interface{})
		rule["name"] = name
		for key, value := range yaml {
			if "names" == key {
				continue
			}
			rule[key] = value
		}
		rules = append(rules, rule)
	}
	return
}

func NormalizeRoleOptions(yaml interface{}) (value map[string]interface{}, err error) {
	// Normal form of role options is a map with SQL token as key and
	// boolean or int value.
	value = map[string]interface{}{
		"SUPERUSER":        false,
		"INHERIT":          true,
		"CREATEROLE":       false,
		"CREATEDB":         false,
		"LOGIN":            false,
		"REPLICATION":      false,
		"BYPASSRLS":        false,
		"CONNECTION LIMIT": -1,
	}
	knownKeys := maps.Keys(value)

	switch yaml.(type) {
	case string:
		s := yaml.(string)
		tokens := strings.Split(s, " ")
		for _, token := range tokens {
			if "" == token {
				continue
			}
			value[strings.TrimPrefix(token, "NO")] = !strings.HasPrefix(token, "NO")
		}
	case map[string]interface{}:
		maps.Copy(value, yaml.(map[string]interface{}))
	case nil:
		return
	default:
		return nil, fmt.Errorf("bad type: %T", yaml)
	}

	err = CheckSpuriousKeys(&value, knownKeys...)
	return
}
