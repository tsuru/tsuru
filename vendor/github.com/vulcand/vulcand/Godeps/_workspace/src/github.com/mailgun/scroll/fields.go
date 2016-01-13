package scroll

import (
	"net/http"
	"regexp"
	"strconv"
	"time"

	"github.com/vulcand/vulcand/Godeps/_workspace/src/github.com/mailgun/log"
)

// Retrieve a POST request field as a string.
// Returns `MissingFieldError` if requested field is missing.
func GetStringField(r *http.Request, fieldName string) (string, error) {
	if _, ok := r.Form[fieldName]; !ok {
		return "", MissingFieldError{fieldName}
	}
	return r.FormValue(fieldName), nil
}

// Retrieves requested field as a string, allowSet provides input sanitization. If an
// error occurs, returns either a `MissingFieldError` or an `UnsafeFieldError`.
func GetStringFieldSafe(r *http.Request, fieldName string, allowSet AllowSet) (string, error) {
	if _, ok := r.Form[fieldName]; !ok {
		return "", MissingFieldError{fieldName}
	}

	fieldValue := r.FormValue(fieldName)
	err := allowSet.IsSafe(fieldValue)
	if err != nil {
		return "", UnsafeFieldError{fieldName, err.Error()}
	}

	return fieldValue, nil
}

// Retrieve a POST request field as a string.
// If the requested field is missing, returns provided default value.
func GetStringFieldWithDefault(r *http.Request, fieldName, defaultValue string) string {
	if fieldValue, err := GetStringField(r, fieldName); err == nil {
		return fieldValue
	}
	return defaultValue
}

// A multiParamRegex is used to convert Ruby and PHP style array params.
// PHP uses ["param[0]", "param[1]",..] instead of ["param", "param",..]
// Ruby uses ["param[]", "param[]",..]
var multiParamRegex *regexp.Regexp

func init() {
	multiParamRegex = regexp.MustCompile(`^([a-z:]*)\[\d*\]$`)
}

// Retrieve fields with the same name as an array of strings.
func GetMultipleFields(r *http.Request, fieldName string) ([]string, error) {
	var values = []string{}

	for field, value := range r.Form {
		// Strip the square brackets.
		if multiParamRegex.ReplaceAllString(field, "$1") == fieldName {
			values = append(values, value...)
		}
	}

	if len(values) == 0 {
		return []string{}, MissingFieldError{fieldName}
	}

	return values, nil
}

// Retrieve a POST request field as an integer.
// Returns `MissingFieldError` if requested field is missing.
func GetIntField(r *http.Request, fieldName string) (int, error) {
	stringField, err := GetStringField(r, fieldName)
	if err != nil {
		return 0, err
	}
	intField, err := strconv.Atoi(stringField)
	if err != nil {
		return 0, InvalidFormatError{fieldName, stringField}
	}
	return intField, nil
}

// Helper method to retrieve an optional timestamp from POST request field.
// If no timestamp provided, returns current time.
// Returns `InvalidFormatError` if provided timestamp can't be parsed.
func GetTimestampField(r *http.Request, fieldName string) (time.Time, error) {
	if _, ok := r.Form[fieldName]; !ok {
		return time.Now(), MissingFieldError{fieldName}
	}
	parsedTime, err := time.Parse(time.RFC1123, r.FormValue(fieldName))
	if err != nil {
		log.Infof("Failed to convert timestamp %v: %v", r.FormValue(fieldName), err)
		return time.Now(), InvalidFormatError{fieldName, r.FormValue(fieldName)}
	}
	return parsedTime, nil
}

// GetDurationField retrieves a request field as a time.Duration, which is not allowed to be negative.
// Returns `MissingFieldError` if requested field is missing.
func GetDurationField(r *http.Request, fieldName string) (time.Duration, error) {
	s, err := GetStringField(r, fieldName)
	if err != nil {
		return 0, err
	}
	d, err := time.ParseDuration(s)
	if err != nil || d < 0 {
		return 0, InvalidFormatError{fieldName, s}
	}
	return d, nil
}

func HasField(r *http.Request, fieldName string) bool {
	if _, ok := r.Form[fieldName]; !ok {
		return false
	}
	return true
}
