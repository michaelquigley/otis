package state

import (
	"fmt"
	"regexp"
)

var idComponentPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// ValidateIDComponent enforces the project/pass component grammar used by Otis ids.
func ValidateIDComponent(value string) error {
	if !idComponentPattern.MatchString(value) {
		return fmt.Errorf("must match lowercase kebab grammar %q", idComponentPattern.String())
	}
	return nil
}
