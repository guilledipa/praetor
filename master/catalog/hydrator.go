// Package catalog provides functions to hydrate the catalog with facts
// provided by the agent making the request.
package catalog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/guilledipa/praetor/schema"
	"gopkg.in/yaml.v3"
	"reflect"
	"strings"
	"text/template"
)

// HydrateCatalog processes a list of raw JSON resources, hydrating template strings with facts.
func HydrateCatalog(rawResources []json.RawMessage, facts map[string]string) ([]any, error) {
	hydratedResources := make([]any, 0, len(rawResources))

	for i, resData := range rawResources {
		var typeMeta schema.TypeMeta
		if err := yaml.Unmarshal(resData, &typeMeta); err != nil {
			return nil, fmt.Errorf("resource %d: error unmarshalling typeMeta: %w", i, err)
		}

		var hydratedResource any
		var err error

		switch typeMeta.Kind {
		case "File":
			var res schema.File
			if err = yaml.Unmarshal(resData, &res); err == nil {
				err = hydrateStruct(&res.Spec, facts)
				hydratedResource = res
			}
		// Add other resource kinds here as they are defined in schema/
		default:
			// For unknown kinds, try to unmarshal to a generic map and hydrate strings
			var genericRes map[string]any
			if err = yaml.Unmarshal(resData, &genericRes); err == nil {
				err = hydrateGenericMap(genericRes, facts)
				hydratedResource = genericRes
			} else {
				return nil, fmt.Errorf("resource %d: unknown kind '%s' and failed to unmarshal as generic map: %w", i, typeMeta.Kind, err)
			}
		}

		if err != nil {
			return nil, fmt.Errorf("resource %d (%s): hydration error: %w", i, typeMeta.Kind, err)
		}
		hydratedResources = append(hydratedResources, hydratedResource)
	}

	return hydratedResources, nil
}

func hydrateStruct(target any, facts map[string]string) error {
	v := reflect.ValueOf(target).Elem()
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		if f.Kind() == reflect.String {
			orig := f.String()
			hydrated, err := processTemplate(orig, facts)
			if err != nil {
				return fmt.Errorf("field %s: %w", v.Type().Field(i).Name, err)
			}
			if f.CanSet() {
				f.SetString(hydrated)
			}
		}
		// TODO: Handle nested structs/maps/slices if necessary
	}
	return nil
}

func hydrateGenericMap(data map[string]any, facts map[string]string) error {
	for key, value := range data {
		switch val := value.(type) {
		case string:
			hydrated, err := processTemplate(val, facts)
			if err != nil {
				return fmt.Errorf("key %s: %w", key, err)
			}
			data[key] = hydrated
		case map[string]any:
			if err := hydrateGenericMap(val, facts); err != nil {
				return err
			}
		case []any:
			for i, item := range val {
				if itemMap, ok := item.(map[string]any); ok {
					if err := hydrateGenericMap(itemMap, facts); err != nil {
						return err
					}
				} else if itemStr, ok := item.(string); ok {
					hydrated, err := processTemplate(itemStr, facts)
					if err != nil {
						return fmt.Errorf("list item %d: %w", i, err)
					}
					val[i] = hydrated
				}
			}
		}
	}
	return nil
}

func processTemplate(tmplStr string, facts map[string]string) (string, error) {
	if !strings.Contains(tmplStr, "{{") {
		return tmplStr, nil // No template markers
	}

	tmpl, err := template.New("hydrate").Parse(tmplStr)
	if err != nil {
		return tmplStr, fmt.Errorf("template parse error: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, map[string]any{"facts": facts}); err != nil {
		return tmplStr, fmt.Errorf("template execute error: %w", err)
	}
	return buf.String(), nil
}
