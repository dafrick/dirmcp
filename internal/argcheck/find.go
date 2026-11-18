package argcheck

import (
	"regexp"
	"strings"
)

// find has its own syntax: leading options, starting points, then an
// expression of primaries (each with a fixed number of arguments) and
// operators. Only the primaries below are allowed; -exec, -delete and the
// -fprint family are not (see denied).
var findPrimaries = map[string]int{
	// tests
	"-name": 1, "-iname": 1, "-path": 1, "-ipath": 1, "-wholename": 1, "-iwholename": 1,
	"-regex": 1, "-iregex": 1, "-lname": 1, "-ilname": 1,
	"-type": 1, "-xtype": 1, "-size": 1, "-empty": 0, "-links": 1, "-inum": 1, "-samefile": 1,
	"-perm": 1, "-user": 1, "-group": 1, "-uid": 1, "-gid": 1, "-nouser": 0, "-nogroup": 0,
	"-readable": 0, "-writable": 0, "-executable": 0,
	"-mtime": 1, "-mmin": 1, "-atime": 1, "-amin": 1, "-ctime": 1, "-cmin": 1, "-used": 1,
	"-newer": 1, "-anewer": 1, "-cnewer": 1,
	"-true": 0, "-false": 0, "-fstype": 1, "-context": 1,
	// actions that only print to stdout
	"-print": 0, "-print0": 0, "-printf": 1, "-ls": 0, "-prune": 0, "-quit": 0,
	// options
	"-maxdepth": 1, "-mindepth": 1, "-depth": 0, "-d": 0, "-mount": 0, "-xdev": 0,
	"-daystart": 0, "-follow": 0, "-noleaf": 0, "-ignore_readdir_race": 0,
	"-noignore_readdir_race": 0, "-regextype": 1, "-warn": 0, "-nowarn": 0,
	"-files0-from": 1, "-help": 0, "--help": 0, "-version": 0, "--version": 0,
	// operators
	"-not": 0, "-a": 0, "-and": 0, "-o": 0, "-or": 0,
}

// -newerXY, e.g. -newermt '2026-01-01'.
var newerXY = regexp.MustCompile(`^-newer[aBcmt][aBcmt]$`)

// Leading options before the starting points.
var findLeading = regexp.MustCompile(`^-([HLP]+|O[0-3])$`)

func checkFind(args []string) error {
	i := 0
	for i < len(args) && findLeading.MatchString(args[i]) {
		i++
	}
	if i < len(args) && args[i] == "-D" {
		return &Error{"find", "-D", "debug options are not allowed"}
	}
	for ; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "(" || a == ")" || a == "!" || a == ",":
			continue
		case !strings.HasPrefix(a, "-") || a == "-":
			continue // starting point
		case newerXY.MatchString(a):
			i++
		default:
			n, ok := findPrimaries[a]
			if !ok {
				hint := "see find --help"
				for _, d := range denied["find"] {
					if a == d {
						hint = "it runs programs, deletes or writes files"
					}
				}
				return &Error{"find", a, hint}
			}
			i += n
		}
	}
	if i > len(args) {
		return &Error{"find", args[len(args)-1], "needs a value"}
	}
	return nil
}
