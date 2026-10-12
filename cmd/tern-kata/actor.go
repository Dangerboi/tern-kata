package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func actorArgs(args []string) (string, []string, error) {
	actor := ""
	rest := []string{}
	for i := 0; i < len(args); i++ {
		if args[i] == "--as" {
			if i+1 == len(args) {
				return "", nil, fmt.Errorf("--as requires an actor")
			}
			i++
			actor = args[i]
		} else if strings.HasPrefix(args[i], "--as=") {
			actor = strings.TrimPrefix(args[i], "--as=")
		} else {
			rest = append(rest, args[i])
		}
	}
	return actor, rest, nil
}
func resolveActor(override string) string {
	if override != "" {
		return override
	}
	for _, key := range []string{"KATA_AUTHOR", "USER"} {
		if value := os.Getenv(key); value != "" {
			return value
		}
	}
	if value, err := exec.Command("git", "config", "user.name").Output(); err == nil && strings.TrimSpace(string(value)) != "" {
		return strings.TrimSpace(string(value))
	}
	return "anonymous"
}
func actorEnvironment(override string) []string {
	env := os.Environ()
	if override == "" {
		return env
	}
	clean := make([]string, 0, len(env)+1)
	for _, entry := range env {
		if !strings.HasPrefix(entry, "KATA_AUTHOR=") {
			clean = append(clean, entry)
		}
	}
	return append(clean, "KATA_AUTHOR="+override)
}
