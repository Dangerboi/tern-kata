package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/Dangerboi/tern-kata/internal/model"
)

// workerForms are the operands `tern-kata worker` takes first.
var workerForms = []string{"init", "add", "edit", "status", "stop", "doctor", "resume", "abandon"}

// writeCompletion is `tern-kata completion <shell>`: the script that completes tern-kata's subcommands, worker forms and
// flags in that shell. Where nothing fits (attach's files) the shell completes file names.
func writeCompletion(shell string, out io.Writer) error {
	script := map[string]func() string{"bash": bashCompletion, "zsh": zshCompletion, "fish": fishCompletion}[shell]
	_, err := io.WriteString(out, script())
	return err
}

func dashed(names []string) string {
	flags := make([]string, len(names))
	for i, name := range names {
		flags[i] = "--" + name
	}
	return strings.Join(flags, " ")
}

func bashCompletion() string {
	return fmt.Sprintf(`# tern-kata completion for bash. Load it with: source <(tern-kata completion bash)
_tern_kata() {
	[[ -n $ZSH_VERSION ]] && setopt localoptions ksharrays # zsh's bash layer numbers COMP_WORDS from 1
	local cur=${COMP_WORDS[COMP_CWORD]} words=""
	if [[ $COMP_CWORD -eq 1 ]]; then
		words="%[1]s --issue --as --version --help"
	else
		case ${COMP_WORDS[1]} in
		attach | detach | run) [[ $cur == -* ]] && words="--as" ;;
		recur) [[ $cur == -* ]] && words="%[2]s --stop --as" ;;
		completion) [[ $COMP_CWORD -eq 2 ]] && words="%[3]s" ;;
		worker)
			if [[ $COMP_CWORD -eq 2 ]]; then
				words="%[4]s --takeover"
			else
				case ${COMP_WORDS[2]} in
				init) words="%[5]s" ;;
				status) words="--json" ;;
				stop) words="--now" ;;
				resume | abandon) [[ $cur == -* ]] && words="--as" ;;
				esac
			fi
			;;
		*) [[ $cur == -* ]] && words="--issue --as" ;;
		esac
	fi
	if [[ -n $words ]]; then
		COMPREPLY=($(compgen -W "$words" -- "$cur"))
	fi
}
complete -o default -F _tern_kata tern-kata
`, strings.Join(subcommands, " "), dashed(model.RecurFlagNames), strings.Join(completionShells, " "), strings.Join(workerForms, " "), dashed(initFlagNames))
}

// zshCompletion is the bash script run through zsh's bash completion layer, so the two shells offer the same words.
func zshCompletion() string {
	return `# tern-kata completion for zsh. Load it with: source <(tern-kata completion zsh)
(( $+functions[compdef] )) || { autoload -U +X compinit && compinit }
autoload -U +X bashcompinit && bashcompinit
` + strings.SplitN(bashCompletion(), "\n", 2)[1]
}

func fishCompletion() string {
	var b strings.Builder
	line := func(format string, args ...any) { fmt.Fprintf(&b, "complete -c tern-kata "+format+"\n", args...) }
	b.WriteString("# tern-kata completion for fish. Load it with: tern-kata completion fish | source\n")
	line("-n __fish_use_subcommand -f -a '%s'", strings.Join(subcommands, " "))
	line("-n __fish_use_subcommand -l issue -r -d 'open this Issue first'")
	line("-l as -r -d 'author for edits'")
	line("-l version -d 'print the version and exit'")
	line("-s h -l help -d 'print the help and exit'")
	line("-n '__fish_seen_subcommand_from detach recur run completion worker' -f")
	line("-n '__fish_seen_subcommand_from completion' -a '%s'", strings.Join(completionShells, " "))
	for _, name := range model.RecurFlagNames {
		line("-n '__fish_seen_subcommand_from recur' -l %s -r", name)
	}
	line("-n '__fish_seen_subcommand_from recur' -l stop -d 'stop the Recurrence'")
	line("-n '__fish_seen_subcommand_from worker; and not __fish_seen_subcommand_from %[1]s' -a '%[1]s'", strings.Join(workerForms, " "))
	line("-n '__fish_seen_subcommand_from worker; and not __fish_seen_subcommand_from %s' -l takeover -d 'claim the Projects leases'", strings.Join(workerForms, " "))
	for _, name := range initFlagNames {
		line("-n '__fish_seen_subcommand_from init' -l %s -r", name)
	}
	line("-n '__fish_seen_subcommand_from status' -l json -d 'the status as JSON'")
	line("-n '__fish_seen_subcommand_from stop' -l now -d 'end the worker at once'")
	return b.String()
}
