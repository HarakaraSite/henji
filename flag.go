package main

import (
	"fmt"
	"regexp"
	"strings"
)

// singleFileValue rejects repeated --file flags instead of silently keeping
// only the last path, which makes shell glob mistakes visible immediately.
type singleFileValue struct {
	path *string
	set  bool
}

func (v *singleFileValue) Set(path string) error {
	if v.set {
		return fmt.Errorf("--file may be specified only once")
	}
	if path == "" {
		return fmt.Errorf("--file requires a non-empty path")
	}
	*v.path = path
	v.set = true
	return nil
}

func (v *singleFileValue) String() string {
	if v == nil || v.path == nil {
		return ""
	}
	return *v.path
}

func (*singleFileValue) Type() string { return "path" }

func newFlagParseError(err error) flagParseError {
	var reason, flag string
	s := err.Error()
	switch {
	case strings.HasPrefix(s, "flag needs an argument:"):
		reason = "Flag %s needs an argument."
		ps := strings.Split(s, "-")
		switch len(ps) {
		case 2: //nolint:mnd
			flag = "-" + ps[len(ps)-1]
		case 3: //nolint:mnd
			flag = "--" + ps[len(ps)-1]
		}
	case strings.HasPrefix(s, "unknown flag:"):
		reason = "Flag %s is missing."
		flag = strings.TrimPrefix(s, "unknown flag: ")
	case strings.HasPrefix(s, "unknown shorthand flag:"):
		reason = "Short flag %s is missing."
		re := regexp.MustCompile(`unknown shorthand flag: '.*' in (-\w)`)
		parts := re.FindStringSubmatch(s)
		if len(parts) > 1 {
			flag = parts[1]
		}
	case strings.HasPrefix(s, "invalid argument"):
		reason = "Flag %s have an invalid argument."
		re := regexp.MustCompile(`invalid argument ".*" for "(.*)" flag: .*`)
		parts := re.FindStringSubmatch(s)
		if len(parts) > 1 {
			flag = parts[1]
		}
	default:
		reason = s
	}
	return flagParseError{
		err:    err,
		reason: reason,
		flag:   flag,
	}
}

type flagParseError struct {
	err    error
	reason string
	flag   string
}

func (f flagParseError) Error() string {
	return f.err.Error()
}

func (f flagParseError) ReasonFormat() string {
	return f.reason
}

func (f flagParseError) Flag() string {
	return f.flag
}
