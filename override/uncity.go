/*
   Copyright 2020 The Compose Specification Authors.

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package override

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/compose-spec/compose-go/v2/format"
	"github.com/compose-spec/compose-go/v2/tree"
)

type indexer func(any, tree.Path) (string, error)

// mergeSpecials defines the custom rules applied by compose when merging yaml trees
var unique = map[tree.Path]indexer{}

func init() {
	unique["services.*.environment"] = keyValueIndexer
	unique["services.*.build.args"] = keyValueIndexer
	unique["services.*.volumes"] = volumeIndexer
	unique["services.*.expose"] = exposeIndexer
	unique["services.*.secrets"] = mountIndexer("/run/secrets")
	unique["services.*.configs"] = mountIndexer("")
	unique["services.*.ports"] = portIndexer
}

// EnforceUnicity removes redefinition of elements declared in a sequence
func EnforceUnicity(value map[string]any) (map[string]any, error) {
	uniq, err := enforceUnicity(value, tree.NewPath())
	if err != nil {
		return nil, err
	}
	return uniq.(map[string]any), nil
}

func enforceUnicity(value any, p tree.Path) (any, error) {
	switch v := value.(type) {
	case map[string]any:
		for k, e := range v {
			u, err := enforceUnicity(e, p.Next(k))
			if err != nil {
				return nil, err
			}
			v[k] = u
		}
		return v, nil
	case []any:
		for pattern, indexer := range unique {
			if p.Matches(pattern) {
				seq := []any{}
				keys := map[string]int{}
				for i, entry := range v {
					key, err := indexer(entry, p.Next(fmt.Sprintf("[%d]", i)))
					if err != nil {
						return nil, err
					}
					if j, ok := keys[key]; ok {
						seq[j] = entry
					} else {
						seq = append(seq, entry)
						keys[key] = len(seq) - 1
					}
				}
				return seq, nil
			}
		}
	}
	return value, nil
}

func keyValueIndexer(y any, _ tree.Path) (string, error) {
	value := y.(string)
	key, _, found := strings.Cut(value, "=")
	if !found {
		return value, nil
	}
	return key, nil
}

func volumeIndexer(y any, p tree.Path) (string, error) {
	switch value := y.(type) {
	case map[string]any:
		target, ok := value["target"].(string)
		if !ok {
			return "", fmt.Errorf("service volume %s is missing a mount target", p)
		}
		return target, nil
	case string:
		volume, err := format.ParseVolume(value)
		if err != nil {
			return "", err
		}
		return volume.Target, nil
	}
	return "", nil
}

func exposeIndexer(a any, path tree.Path) (string, error) {
	switch v := a.(type) {
	case string:
		return v, nil
	case int:
		return strconv.Itoa(v), nil
	default:
		return "", fmt.Errorf("%s: unsupported expose value %s", path, a)
	}
}

func mountIndexer(defaultPath string) indexer {
	return func(a any, path tree.Path) (string, error) {
		switch v := a.(type) {
		case string:
			return fmt.Sprintf("%s/%s", defaultPath, v), nil
		case map[string]any:
			t, ok := v["target"]
			if ok {
				return t.(string), nil
			}
			return fmt.Sprintf("%s/%s", defaultPath, v["source"]), nil
		default:
			return "", fmt.Errorf("%s: unsupported expose value %s", path, a)
		}
	}
}

func portIndexer(y any, p tree.Path) (string, error) {
	switch value := y.(type) {
	case int:
		return strconv.Itoa(value), nil
	case map[string]any:
		target, ok := value["target"]
		if !ok {
			return "", fmt.Errorf("service ports %s is missing a target port", p)
		}
		published, ok := value["published"]
		if !ok {
			// try to parse it as an int
			if pub, ok := value["published"]; ok {
				published = fmt.Sprintf("%d", pub)
			}
		}
		host, ok := value["host_ip"]
		if !ok {
			host = "0.0.0.0"
		}
		protocol, ok := value["protocol"]
		if !ok {
			protocol = "tcp"
		}
		return fmt.Sprintf("%s:%s:%d/%s", host, published, target, protocol), nil
	case string:
		return value, nil
	}
	return "", nil
}
