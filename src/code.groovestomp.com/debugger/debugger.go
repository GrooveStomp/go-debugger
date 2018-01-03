package main

import (
	"flag"
	"log"
	"os"
	"os/exec"
	"syscall"
)

func main() {
	flag.Parse()
	input := flag.Arg(0)

	cmd := exec.Command(input)
	cmd.Args = []string{input}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Ptrace:true}
	err := cmd.Start()
	if err != nil {
		log.Fatal(err)
	}

	err = cmd.Wait()
	log.Printf("State: %v\n", err)
	wpid := cmd.Process.Pid
	pgid, err := syscall.Getpgid(wpid)
	if err != nil {
		log.Panic(err)
	}

	err = syscall.PtraceSetOptions(wpid, syscall.PTRACE_O_TRACECLONE)
	if err != nil {
		log.Fatal(err)
	}

	err = syscall.PtraceSingleStep(wpid)
	if err != nil {
		log.Fatal(err)
	}

	steps := 1

	for {
		var ws syscall.WaitStatus
		wpid, err = syscall.Wait4(-1 * pgid, &ws, syscall.WALL, nil)
		if wpid == -1 {
			log.Fatal(err)
		}

		if wpid == cmd.Process.Pid && ws.Exited() {
			break
		}

		if !ws.Exited() {
			err := syscall.PtraceSingleStep(wpid)
			if err != nil {
				log.Fatal(err)
			}
			steps += 1
		}
	}

	log.Printf("Steps: %d\n", steps)
}
