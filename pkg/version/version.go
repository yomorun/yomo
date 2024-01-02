// Package version provides functionality for parsing versions..
package version

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Version is used by the source/sfn to communicate their version to the server.
type Version struct {
	Major int
	Minor int
	Patch int
}

// Parse parses a string into a Version. The string format must follow the `Major.Minor.Patch`
// formatting, and the Major, Minor, and Patch components must be numeric. If they are not,
// a parse error will be returned.
func Parse(str string) (*Version, error) {
	if str == "" {
		return nil, errors.New("empty version string")
	}
	vs := strings.Split(str, ".")
	if len(vs) != 3 {
		return nil, fmt.Errorf("invalid semantic version, params=%s", str)
	}

	major, err := strconv.Atoi(vs[0])
	if err != nil {
		return nil, fmt.Errorf("invalid version major, params=%s", str)
	}

	minor, err := strconv.Atoi(vs[1])
	if err != nil {
		return nil, fmt.Errorf("invalid version minor, params=%s", str)
	}

	patch, err := strconv.Atoi(vs[2])
	if err != nil {
		return nil, fmt.Errorf("invalid version patch, params=%s", str)
	}

	ver := &Version{Major: major, Minor: minor, Patch: patch}

	return ver, nil
}
