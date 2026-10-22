// Command stub is a fake omp for the worker's tests. It accepts omp's headless flags, prints omp-style JSON lines
// and exits with a chosen status or hangs. Its own flags (all start with --stub-) say what to do:
//
//	--stub-exit N          exit status (default 0)
//	--stub-text T          the final assistant message (default "done")
//	--stub-stop REASON     stopReason of that message (default "stop")
//	--stub-no-end          print no agent_end
//	--stub-hang            print the session line, then wait for a signal
//	--stub-log FILE        append one JSON line describing this launch
//	--stub-hang-first      like --stub-hang, but only when not started with --resume (so a resumed attempt finishes)
//	--stub-linger          after agent_end, keep running until signalled (the process ended its turn but has not exited)
//	--stub-error-status N  end the turn with a provider failure: stopReason error, errorStatus N, exit 1
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

func main() {
	var (
		exit      = 0
		text      = "done"
		stop      = "stop"
		noEnd     bool
		hang      bool
		hangFirst bool
		linger    bool
		errStatus int
		logPath   string
		sessionIn string
		resume    string
		prompt    string
	)
	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		switch a := args[i]; a {
		case "--stub-exit":
			i++
			exit, _ = strconv.Atoi(args[i])
		case "--stub-text":
			i++
			text = args[i]
		case "--stub-stop":
			i++
			stop = args[i]
		case "--stub-log":
			i++
			logPath = args[i]
		case "--stub-no-end":
			noEnd = true
		case "--stub-hang":
			hang = true
		case "--stub-hang-first":
			hangFirst = true
		case "--stub-linger":
			linger = true
		case "--stub-error-status":
			i++
			errStatus, _ = strconv.Atoi(args[i])
		case "--session-dir":
			i++
			sessionIn = args[i]
		case "--resume":
			i++
			resume = args[i]
		case "--mode", "--model", "--thinking", "--approval-mode":
			i++
		default:
			if len(a) > 1 && a[0] == '@' {
				raw, _ := os.ReadFile(a[1:])
				prompt = string(raw)
			}
		}
	}
	if logPath != "" {
		cwd, _ := os.Getwd()
		stdin := make(chan string, 1)
		go func() {
			buf := make([]byte, 1)
			n, _ := os.Stdin.Read(buf)
			if n == 0 {
				stdin <- "eof"
			} else {
				stdin <- "data"
			}
		}()
		state := "open"
		select {
		case state = <-stdin:
		case <-time.After(time.Second):
		}
		line, _ := json.Marshal(map[string]any{"args": os.Args[1:], "cwd": cwd, "author": os.Getenv("KATA_AUTHOR"), "secret": os.Getenv("STUB_SECRET"), "prompt": prompt, "stdin": state, "resume": resume})
		if f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
			_, _ = f.Write(append(line, '\n'))
			_ = f.Close()
		}
	}
	id := "00000000-0000-7000-8000-000000000001"
	if sessionIn != "" {
		_ = os.MkdirAll(sessionIn, 0o755)
		if existing, _ := filepath.Glob(filepath.Join(sessionIn, "*_"+id+".jsonl")); len(existing) == 0 { // omp appends to the resumed session's file
			name := fmt.Sprintf("%s_%s.jsonl", time.Now().UTC().Format("2006-01-02T15-04-05-000Z"), id)
			header, _ := json.Marshal(map[string]any{"type": "session", "id": id})
			_ = os.WriteFile(filepath.Join(sessionIn, name), append(header, '\n'), 0o644)
		}
	}
	fmt.Printf(`{"type":"session","version":3,"id":%q,"timestamp":"2026-10-22T00:00:00.000Z","cwd":"/tmp"}`+"\n", id)
	fmt.Println(`{"type":"agent_start"}`)
	fmt.Println(`{"type":"surprise","unknown":true}`)
	fmt.Println("not json at all")
	if hang || (hangFirst && resume == "") {
		time.Sleep(time.Hour)
	}
	msg := map[string]any{
		"role": "assistant", "content": []any{map[string]any{"type": "thinking", "text": "hm"}, map[string]any{"type": "text", "text": text}},
		"provider": "stub", "model": "stub-1", "stopReason": stop,
		"usage": map[string]any{"input": 10, "output": 2, "totalTokens": 12},
	}
	if errStatus != 0 {
		msg["content"], msg["stopReason"], msg["errorStatus"], msg["errorMessage"] = []any{}, "error", errStatus, fmt.Sprintf("%d provider says no", errStatus)
		exit = 1
	}
	message, _ := json.Marshal(msg)
	fmt.Printf(`{"type":"message_end","message":%s}`+"\n", message)
	if !noEnd {
		fmt.Printf(`{"type":"agent_end","messages":[{"role":"user","content":[{"type":"text","text":"p"}]},%s],"isTerminal":true,"yielded":true}`+"\n", message)
	}
	if linger {
		time.Sleep(time.Hour)
	}
	os.Exit(exit)
}
