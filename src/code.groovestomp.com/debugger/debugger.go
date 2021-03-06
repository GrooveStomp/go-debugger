package main

import (
	"bufio"
	"debug/elf"
	"debug/gosym"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

var (
	breakpoints  map[string][]int
	pcSourceLine int
	pcSourceFile string
)

func initTracee(path string) int {
	cmd := exec.Command(path)
	cmd.Args = []string{path}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Ptrace: true}
	err := cmd.Start()
	if err != nil {
		log.Fatal(err)
	}

	returnStatus := cmd.Wait()
	if returnStatus == nil {
		log.Fatal("Program exited")
	}

	return cmd.Process.Pid
}

func step(pid int) *syscall.WaitStatus {
	err := syscall.PtraceSingleStep(pid)
	if err != nil {
		log.Fatal(err)
	}

	var ws syscall.WaitStatus
	_, err = syscall.Wait4(pid, &ws, syscall.WALL, nil)
	if err != nil {
		log.Fatal(err)
	}

	return &ws
}

func cont(pid int) *syscall.WaitStatus {
	err := syscall.PtraceCont(pid, 0)
	if err != nil {
		log.Fatal(err)
	}

	var ws syscall.WaitStatus
	_, err = syscall.Wait4(pid, &ws, syscall.WALL, nil)
	if err != nil {
		log.Fatal(err)
	}

	return &ws
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

func main() {
	breakpoints = make(map[string][]int)
	flag.Parse()
	filepath := flag.Arg(0)
	exe, err := elf.Open(filepath)
	if err != nil {
		log.Fatal(err)
	}
	defer exe.Close()

	pid := initTracee(filepath)

	symbolTable := getSymbolTable(exe)
	symbol := symbolTable.LookupFunc("main.main")
	filename, lineno, _ := symbolTable.PCToLine(symbol.Entry)

	runToSourceLine(pid, filename, lineno, symbolTable)

	pc := getPC(pid)
	showListing(filename, lineno)

	for {
		reader := bufio.NewReader(os.Stdin)
		fmt.Print("> ")
		command, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				fmt.Println()
				break
			}
			log.Fatal(err)
		}
		command = command[:len(command)-1] // Strip trailing newline.

		if isHelpCommand(command) {
			showHelp()
		} else if isBreakpointCommand(command) {
			filename, lineNumber, err := parseBreakpointCommand(command, filename)
			if err != nil {
				log.Fatal(err)
			}

			breakpoints[filename] = append(breakpoints[filename], lineNumber)

			pc, _, err := symbolTable.LineToPC(filename, lineNumber)
			if err != nil {
				log.Fatal(err)
			}

			setBreakpoint(pid, uintptr(pc))
			showListing(filename, lineNumber)

		} else if isStepIntoCommand(command) {
			step(pid)
			pc = getPC(pid)
			filename, lineno, _ := symbolTable.PCToLine(pc)
			showListing(filename, lineno - 1)
		} else if isStepOverCommand(command) {
			lineno = lineno + 1
			status := runToSourceLine(pid, filename, lineno, symbolTable)
			if status.Exited() {
				break
			}
			showListing(filename, lineno)
		} else if isContinueCommand(command) {
			status := cont(pid)
			if status.Exited() {
				break
			} else {
				pc = getPC(pid)
				filename, lineno, _ := symbolTable.PCToLine(pc)
				pcSourceLine = lineno
				pcSourceFile = filename
				showListing(filename, lineno)
			}
		} else if isListingCommand(command) {
			pc = getPC(pid)
			filename, lineno, _ = symbolTable.PCToLine(pc)

			parts := strings.Split(command, " ")
			if len(parts) == 2 {
				lineno, err = strconv.Atoi(parts[len(parts)-1])
				if err != nil {
					log.Fatal(err)
				}
			}

			showListing(filename, lineno)
		} else if isQuitCommand(command) {
			process, err := os.FindProcess(pid)
			if err != nil {
				log.Fatal(err)
			}
			process.Kill()
			break
		} else {
			fmt.Println("command unknown")
		}
	}
}

func isBreakpointCommand(command string) bool {
	return strings.HasPrefix(command, "breakpoint ") ||
		strings.HasPrefix(command, "break ") ||
		strings.HasPrefix(command, "b ")
}

func isStepIntoCommand(command string) bool {
	return command == "step" || command == "s"
}

func isStepOverCommand(command string) bool {
	return command == "next" || command == "n"
}

func isContinueCommand(command string) bool {
	return command == "continue" || command == "c"
}

func isHelpCommand(command string) bool {
	return command == "help" || command == "h" || command == "?"
}

func isListingCommand(command string) bool {
	return strings.HasPrefix(command, "listing ") ||
		strings.HasPrefix(command, "list ") ||
		strings.HasPrefix(command, "l ") ||
		command == "listing" ||
		command == "list" ||
		command == "l"
}

func isQuitCommand(command string) bool {
	return command == "q" || command == "quit" || command == "exit"
}

func showHelp() {
	text := `
Set Breakpoint

  b <location>
  break <location>
  breakpoint <location>

  <location> is the name of a function or a line number.

Step

  Steps into the next machine instruction.

  s
  step

Next Source Line

  Steps to the next source code line.

  n
  next

Continue

  c
  continue

Listing

  Display source code centered around the current instruction.

  l <lineno>
  list <lineno>

  <lineno> is optional; when given the display will be centered around the given
  line number.

Help

  ?
  h
  help

Quit

  q
  quit
  exit
`
	fmt.Println(text)
}

func getSymbolTable(exe *elf.File) *gosym.Table {
	var exeSection *elf.Section

	exeSection = exe.Section(".gopclntab")
	if exeSection == nil {
		log.Fatal("Cannot read .gpclntab section")
	}
	lineTableData, err := exeSection.Data()

	exeSection = exe.Section(".gosymtab")
	if exeSection == nil {
		log.Fatal("Cannot read .gosymtab section")
	}
	symbolTableData, err := exeSection.Data()

	exeSection = exe.Section(".text")
	if exeSection == nil {
		log.Fatal("Cannot read .text section")
	}
	textSectionAddress := exeSection.Addr

	lineTable := gosym.NewLineTable(lineTableData, textSectionAddress)
	symbolTable, err := gosym.NewTable(symbolTableData, lineTable)
	if err != nil {
		log.Fatalf("Cannot create symbol table: %v", err)
	}

	return symbolTable
}

func showListing(filename string, lineNumber int) {
	fileBytes, err := ioutil.ReadFile(filename)
	if err != nil {
		log.Fatal(err)
	}
	fstring := string(fileBytes)
	lines := strings.Split(fstring, "\n")

	start := lineNumber - 4
	if start < 0 {
		start = 0
	}
	end := lineNumber + 3
	if end >= len(lines) {
		end = len(lines) - 1
	}

	fmt.Println()
	for i := start; i < end; i++ {

		isBreakpoint := false
		for j := 0; j < len(breakpoints[filename]); j++ {
			if breakpoints[filename][j] == i + 1 {
				isBreakpoint = true
			}
		}

		if (i + 1) == pcSourceLine && filename == pcSourceFile {
			fmt.Print("> ")
		} else if isBreakpoint {
			fmt.Print("* ")
		} else {
			fmt.Print("  ")
		}
		fmt.Printf("%v %v\n", i+1, lines[i])
	}
	fmt.Println()
}

func runToSourceLine(pid int, filename string, lineNumber int, symbolTable *gosym.Table) *syscall.WaitStatus {
	pc, _, err := symbolTable.LineToPC(filename, lineNumber)
	if err != nil {
		log.Fatal(err)
	}

	original := setBreakpoint(pid, uintptr(pc))
	status := cont(pid)
	clearBreakpoint(pid, uintptr(pc), original)
	setPC(pid, uint64(pc))
	pcSourceLine = lineNumber
	pcSourceFile = filename

	return status
}

func parseBreakpointCommand(command string, filename string) (string, int, error) {
	parts := strings.Split(command, " ")
	command = parts[len(parts)-1]

	var num string

	if strings.Contains(command, ":") {
		parts = strings.Split(parts[len(parts)-1], ":")
		filename = parts[0]
		num = parts[1]
	} else {
		num = command
	}

	lineNumber, err := strconv.Atoi(num)
	if err != nil {
		return "", -1, err
	}

	return filename, lineNumber, nil
}
