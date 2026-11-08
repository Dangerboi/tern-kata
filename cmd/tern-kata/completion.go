package main

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/Dangerboi/tern-kata/internal/model"
)

var docbankForms = []string{"setup", "supervised", "supervise"}

var workerForms = []string{"init", "add", "edit", "upgrade", "status", "stop", "supervised", "doctor", "resume", "abandon"}

var viewsForms = []string{"show", "add", "rm"}

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
		migrate-attachments) [[ $cur == -* ]] && words="--as" ;;
		agents) [[ $cur == -* ]] && words="--issue" ;;
		run | team) ;;
		message) [[ $cur == -* ]] && words="--as --re" ;;
		recur) [[ $cur == -* ]] && words="%[2]s --stop --as" ;;
		views)
			if [[ $COMP_CWORD -eq 2 ]]; then
				words="%[6]s"
			else
				case ${COMP_WORDS[2]} in
				show) [[ $cur == -* ]] && words="--json --as" ;;
				add) [[ $cur == -* ]] && words="%[7]s --replace" ;;
				esac
			fi
			;;
		update) [[ $cur == -* ]] && words="--force --pause" ;;
		agent)
			if [[ $cur == -* ]]; then
				words="%[8]s"
				local i=0 w
				for w in "${COMP_WORDS[@]}"; do # after the lone -- the words are the command's
					[[ $i -ge $COMP_CWORD ]] && break
					[[ $i -ge 2 && $w == -- ]] && words=""
					i=$((i + 1))
				done
			fi
			;;
		docbank)
			if [[ $cur == -* ]]; then
				[[ ${COMP_WORDS[2]} == setup ]] && words="%[9]s"
			elif [[ $COMP_CWORD -eq 2 ]]; then
				words="%[10]s"
			elif [[ $COMP_CWORD -eq 3 && ${COMP_WORDS[2]} == supervise ]]; then
				words="on off"
			fi
			;;
		completion) [[ $COMP_CWORD -eq 2 ]] && words="%[3]s" ;;
		worker)
			if [[ $COMP_CWORD -eq 2 ]]; then
				words="%[4]s --takeover --as"
			else
				case ${COMP_WORDS[2]} in
				init) words="%[5]s --as" ;;
				status) words="--json" ;;
				stop) words="--now" ;;
				doctor | resume | abandon) [[ $cur == -* ]] && words="--as" ;;
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
`, strings.Join(subcommands, " "), dashed(model.RecurFlagNames), strings.Join(completionShells, " "), strings.Join(workerForms, " "), dashed(initFlagNames), strings.Join(viewsForms, " "), dashed(viewFlagNames), dashed(flagNames(agentFlags)), dashed(flagNames(docbankFlags)), strings.Join(docbankForms, " "))
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
	line("-n __fish_use_subcommand -l as -r -d 'author for edits'")
	line("-n '__fish_seen_subcommand_from migrate-attachments recur message' -l as -r -d 'author for edits'")
	line("-n '__fish_seen_subcommand_from views; and __fish_seen_subcommand_from show' -l as -r -d 'author for edits'")
	line("-n '__fish_seen_subcommand_from worker; and not __fish_seen_subcommand_from add edit upgrade status stop supervised' -l as -r -d 'author for edits'")
	line("-l version -d 'print the version and exit'")
	line("-s h -l help -d 'print the help and exit'")
	line("-n '__fish_seen_subcommand_from migrate-attachments recur run views completion worker update agent agents team message' -f")
	line("-n '__fish_seen_subcommand_from agents' -l issue -r -d 'only this Issue'")
	line("-n '__fish_seen_subcommand_from message' -l re -r -d 'comment the message asks the agent to answer'")
	fishTableFlags(line, agentFlags, "agent; and not contains -- -- (commandline -opc)") // after the lone -- the words are the command's, which bash completion leaves alone too
	line("-n '__fish_seen_subcommand_from docbank; and not __fish_seen_subcommand_from %[1]s' -f -a '%[1]s'", strings.Join(docbankForms, " "))
	line("-n '__fish_seen_subcommand_from supervise; and not __fish_seen_subcommand_from on off' -f -a 'on off'")
	fishTableFlags(line, docbankFlags, "setup")
	line("-n '__fish_seen_subcommand_from update' -l force -d 'install even when a check refuses'")
	line("-n '__fish_seen_subcommand_from update' -l pause -d 'wait for Enter after a success'")
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
	line("-n '__fish_seen_subcommand_from views; and not __fish_seen_subcommand_from %[1]s' -a '%[1]s'", strings.Join(viewsForms, " "))
	for _, name := range viewFlagNames {
		line("-n '__fish_seen_subcommand_from views' -l %s -r", name)
	}
	line("-n '__fish_seen_subcommand_from views' -l replace -d 'save over a view of that name'")
	line("-n '__fish_seen_subcommand_from status show' -l json -d 'the status or the Issues as JSON'")
	line("-n '__fish_seen_subcommand_from stop' -l now -d 'end the worker at once'")
	return b.String()
}

// fishTableFlags writes one fish completion per flag of table, offered where the subcommand condition holds. skip names a flag
// another line already offers.
func fishTableFlags(line func(string, ...any), table []tableFlag, condition string, skip ...string) {
	for _, f := range table {
		if slices.Contains(skip, f.name) {
			continue
		}
		value := ""
		if f.takesValue {
			value = " -r"
		}
		line("-n '__fish_seen_subcommand_from %s' -l %s%s -d '%s'", condition, f.name, value, strings.ReplaceAll(f.what, "'", `\'`))
	}
}
