package env

import (
	"os"
	"strconv"
)

// GetBool gets the bool value from env
func GetBool(key string, defaultValue bool) bool {
	value := os.Getenv(key)
	if len(value) != 0 {
		flag, err := strconv.ParseBool(value)
		if err != nil {
			return defaultValue
		}
		return flag
	}
	return defaultValue
}

// GetInt gets the int value from env
func GetInt(key string, defaultValue int) int {
	value := os.Getenv(key)
	if len(value) != 0 {
		result, err := strconv.Atoi(value)
		if err != nil {
			return defaultValue
		}

		return result
	}
	return defaultValue
}

// GetString gets the string value from env
func GetString(key string, defaultValue string) string {
	value := os.Getenv(key)
	if len(value) != 0 {
		return value
	}
	return defaultValue
}
