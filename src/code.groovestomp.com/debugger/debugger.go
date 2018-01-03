package main

import (
	"flag"
	"log"
	"os"
	"os/exec"
	"syscall"
)

func step(pid int) {
	err := syscall.PtraceSingleStep(pid)
	if err != nil {
		log.Fatal(err)
	}
}

func cont(pid int) {
	err := syscall.PtraceCont(pid, 0)
	if err != nil {
		log.Fatal(err)
	}
}

func setPC(pid int, pc uint64) {
	var regs syscall.PtraceRegs
	err := syscall.PtraceGetRegs(pid, &regs)
	if err != nil {
		log.Fatal(err)
	}
	regs.SetPC(pc)
	err = syscall.PtraceSetRegs(pid, &regs)
	if err != nil {
		log.Fatal(err)
	}
}

func getPC(pid int) uint64 {
	var regs syscall.PtraceRegs
	err := syscall.PtraceGetRegs(pid, &regs)
	if err != nil {
		log.Fatal(err)
	}
	return regs.PC()
}

func setBreakpoint(pid int, breakpoint uintptr) []byte {
	original := make([]byte, 1)
	_, err := syscall.PtracePeekData(pid, breakpoint, original)
	if err != nil {
		log.Fatal(err)
	}
	_, err = syscall.PtracePokeData(pid, breakpoint, []byte{0xCC})
	if err != nil {
		log.Fatal(err)
	}
	return original
}

func clearBreakpoint(pid int, breakpoint uintptr, original []byte) {
	_, err := syscall.PtracePokeData(pid, breakpoint, original)
	if err != nil {
		log.Fatal(err)
	}
}

func printState(pid int) {
	var regs syscall.PtraceRegs
	err := syscall.PtraceGetRegs(pid, &regs)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("RAX=%d, RDI=%d\n", regs.Rax, regs.Rdi)
}

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
	pid := cmd.Process.Pid
	breakpoint := uintptr(getPC(pid) + 5)
	original := setBreakpoint(pid, breakpoint)
	cont(pid)

	var ws syscall.WaitStatus
	_, err = syscall.Wait4(pid, &ws, syscall.WALL, nil)
	clearBreakpoint(pid, breakpoint, original)
	printState(pid)
	setPC(pid, uint64(breakpoint))
	step(pid)
	_, err = syscall.Wait4(pid, &ws, syscall.WALL, nil)
	printState(pid)
}
