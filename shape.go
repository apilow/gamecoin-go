package gamecoin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"
	"time"
)

// A response that is JSON but not the shape the API promises is reported as CodeInvalidResponse. The check walks the
// target Go type against the decoded JSON: every field the type declares (those without "omitempty") must be present with
// the right type, null being accepted for pointer fields only; extra fields in the response are ignored. Messages name a
// path, never a value.

var timeType = reflect.TypeOf(time.Time{})

// shapeRoot is the first element of the path in a message: the entity for a direct object, "response" for a wrapper.
func shapeRoot(out any) string {
	switch out.(type) {
	case *Catalog:
		return "catalog"
	case *Player:
		return "player"
	case *Wallet:
		return "wallet"
	case *Order:
		return "order"
	}
	return "response"
}

// parseResponse checks the shape of body against the type out points to, then decodes it into out. It returns a short
// reason when the body is not acceptable.
func parseResponse(body []byte, out any) (reason string, cause error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var document any
	if err := decoder.Decode(&document); err != nil {
		return "the body is not JSON", err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return "the body is not JSON", err
	}
	if err := checkShape(document, reflect.TypeOf(out).Elem(), shapeRoot(out)); err != nil {
		return err.Error(), nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return "a field has the wrong type", err
	}
	return "", nil
}

func shapeError(path string, t reflect.Type, nullable bool) error {
	expected := describeType(t)
	if nullable {
		expected += " or null"
	}
	return fmt.Errorf("%s must be %s", path, expected)
}

func describeType(t reflect.Type) string {
	if t.Kind() == reflect.Ptr {
		return describeType(t.Elem())
	}
	if t == timeType {
		return "an ISO 8601 date-time"
	}
	switch t.Kind() {
	case reflect.Struct, reflect.Map:
		return "an object"
	case reflect.Slice:
		return "an array"
	case reflect.String:
		return "a string"
	case reflect.Int64, reflect.Int:
		return "an integer"
	case reflect.Bool:
		return "a boolean"
	}
	return "valid"
}

func checkShape(value any, t reflect.Type, path string) error {
	if t.Kind() == reflect.Ptr {
		if value == nil {
			return nil
		}
		return checkShape(value, t.Elem(), path)
	}

	switch {
	case t == timeType:
		text, ok := value.(string)
		if _, err := time.Parse(time.RFC3339Nano, text); !ok || err != nil {
			return shapeError(path, t, false)
		}
		return nil
	case t.Kind() == reflect.Struct:
		return checkStruct(value, t, path)
	case t.Kind() == reflect.Slice:
		items, ok := value.([]any)
		if !ok {
			return shapeError(path, t, false)
		}
		for i, item := range items {
			if err := checkShape(item, t.Elem(), fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
		return nil
	case t.Kind() == reflect.Map:
		object, ok := value.(map[string]any)
		if !ok {
			return shapeError(path, t, false)
		}
		for key, item := range object {
			if err := checkShape(item, t.Elem(), path+"."+key); err != nil {
				return err
			}
		}
		return nil
	case t.Kind() == reflect.String:
		if _, ok := value.(string); !ok {
			return shapeError(path, t, false)
		}
		return nil
	case t.Kind() == reflect.Int64:
		number, ok := value.(json.Number)
		if !ok {
			return shapeError(path, t, false)
		}
		if _, err := number.Int64(); err != nil {
			return shapeError(path, t, false)
		}
		return nil
	case t.Kind() == reflect.Bool:
		if _, ok := value.(bool); !ok {
			return shapeError(path, t, false)
		}
		return nil
	}
	return nil
}

func checkStruct(value any, t reflect.Type, path string) error {
	object, ok := value.(map[string]any)
	if !ok {
		return shapeError(path, t, false)
	}
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		name, options, _ := strings.Cut(field.Tag.Get("json"), ",")
		if name == "-" || !field.IsExported() {
			continue
		}
		if name == "" {
			name = field.Name
		}
		optional := strings.Contains(options, "omitempty")
		child, present := object[name]
		childPath := path + "." + name
		switch {
		case !present && optional:
			continue
		case !present:
			return shapeError(childPath, field.Type, field.Type.Kind() == reflect.Ptr)
		case child == nil && optional:
			return shapeError(childPath, field.Type, false) // an optional field is absent or set, never null
		}
		if err := checkShape(child, field.Type, childPath); err != nil {
			return err
		}
	}
	return nil
}
