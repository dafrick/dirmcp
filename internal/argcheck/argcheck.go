// Package argcheck decides whether a program may run with given arguments.
//
// Every program has a table of allowed options (generated from its --help,
// see gen/). Options in the denylist below are never allowed, whatever the
// table says. Non-option arguments are not checked: paths are confined by
// Landlock and patterns are data.
package argcheck

import (
	"fmt"
	"sort"
	"strings"
)

// Arity says whether an option takes a value.
type Arity uint8

const (
	None     Arity = iota // -a, --all
	Required              // -n 5, -n5, --lines=5, --lines 5
	Optional              // --color, --color=auto (long form only, "=" required)
)

// Prog is the option table of one program.
type Prog struct {
	Short  map[rune]Arity
	Long   map[string]Arity
	Digits bool     // accepts -NUM (head -5)
	Force  []string // always passed first (e.g. --sandbox)
}

// denied lists options that run other programs, load code, or write outside
// the normal output channel. They are removed from every table.
var denied = map[string][]string{
	"find": {"-exec", "-execdir", "-ok", "-okdir", "-delete", "-fprint", "-fprint0", "-fprintf", "-fls"},
	"rg":   {"--pre", "--pre-glob", "-z", "--search-zip", "--hostname-bin"},
	"sort": {"--compress-program"},
	"gawk": {"-l", "--load", "-E", "--exec", "-M", "--bignum", "-D", "--debug", "-p", "--profile", "-d", "--dump-variables", "-o", "--pretty-print", "-L", "--lint"},
	"file": {"-z", "--uncompress", "-Z", "--uncompress-noreport"},
	"date": {"-s", "--set"},
	"tree": {"-R"},
	"diff": {}, "sed": {}, "grep": {},
}

// Error is a rejected argument with a hint for the caller.
type Error struct {
	Prog, Arg, Hint string
}

func (e *Error) Error() string {
	s := fmt.Sprintf("%s: option %q is not allowed", e.Prog, e.Arg)
	if e.Hint != "" {
		s += "; " + e.Hint
	}
	return s
}

// Table returns the effective table for name, or nil if unknown.
func Table(name string) *Prog {
	if name == "find" {
		return nil // own checker
	}
	p, ok := generated[name]
	if !ok {
		return nil
	}
	return applyDenied(name, p)
}

// Programs returns all allowed program names, sorted.
func Programs() []string {
	var n []string
	for k := range generated {
		n = append(n, k)
	}
	n = append(n, "find", "awk")
	sort.Strings(n)
	return n
}

func applyDenied(name string, p Prog) *Prog {
	out := &Prog{Short: map[rune]Arity{}, Long: map[string]Arity{}, Digits: p.Digits, Force: forced[name]}
	for k, v := range p.Short {
		out.Short[k] = v
	}
	for k, v := range p.Long {
		out.Long[k] = v
	}
	for _, d := range denied[name] {
		if strings.HasPrefix(d, "--") {
			delete(out.Long, d[2:])
		} else if len(d) == 2 {
			delete(out.Short, rune(d[1]))
		}
	}
	return out
}

// forced options, prepended to the program's arguments.
var forced = map[string][]string{
	"sed":  {"--sandbox"},
	"gawk": {"--sandbox"},
	"rg":   {"--no-config"},
}

// Check validates args for program name and returns the full argument list
// to run (forced options first). awk is an alias for gawk.
func Check(name string, args []string) ([]string, error) {
	if name == "awk" {
		name = "gawk"
	}
	if name == "find" {
		if err := checkFind(args); err != nil {
			return nil, err
		}
		return args, nil
	}
	p := Table(name)
	if p == nil {
		return nil, fmt.Errorf("%s: not allowed", name)
	}
	if err := p.check(name, args); err != nil {
		return nil, err
	}
	return append(append([]string{}, p.Force...), args...), nil
}

func (p *Prog) check(name string, args []string) error {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			return nil // the rest is operands
		case a == "-" || !strings.HasPrefix(a, "-"):
			continue // operand
		case strings.HasPrefix(a, "--"):
			key, _, hasVal := strings.Cut(a[2:], "=")
			ar, ok := p.Long[key]
			if !ok {
				return &Error{name, "--" + key, p.closest(key)}
			}
			switch {
			case ar == None && hasVal:
				return &Error{name, a, "takes no value"}
			case ar == Required && !hasVal:
				i++ // value is the next argument
				if i == len(args) {
					return &Error{name, a, "needs a value"}
				}
			}
		default:
			if p.Digits && allDigits(a[1:]) {
				continue
			}
			for j, r := range a[1:] {
				ar, ok := p.Short[r]
				if !ok {
					return &Error{name, "-" + string(r), p.closest(string(r))}
				}
				if ar == Required {
					if j+1 == len(a[1:]) { // value is the next argument
						i++
						if i == len(args) {
							return &Error{name, "-" + string(r), "needs a value"}
						}
					}
					break // the rest of a is the value
				}
			}
		}
	}
	return nil
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// closest suggests allowed options that share a prefix with key.
func (p *Prog) closest(key string) string {
	var m []string
	for k := range p.Long {
		if len(key) > 1 && (strings.HasPrefix(k, key[:2]) || strings.HasPrefix(key, k)) {
			m = append(m, "--"+k)
		}
	}
	sort.Strings(m)
	if len(m) > 8 {
		m = m[:8]
	}
	if len(m) == 0 {
		return "see --help for allowed options"
	}
	return "similar allowed: " + strings.Join(m, " ")
}
