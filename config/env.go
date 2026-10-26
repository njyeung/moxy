package config

import "os"

// EnvOr returns the env variable, or def if it's unset or empty
func EnvOr(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}
