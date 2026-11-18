package shell

import "os/exec"

func mustCmd(name string, args ...string) *exec.Cmd {
	c := exec.Command(name, args...)
	c.Env = []string{}
	return c
}
