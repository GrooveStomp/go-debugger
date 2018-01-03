section .data
    msg db "hello, world!", 0xA
    len equ $ - msg

section .text
    global _start

_start:
    mov rax, 1 ; write syscall (https://linux.die.net/man/2/write)
    mov rdi, 1 ; stdout
    mov rsi, msg
    mov rdx, len
    ; Passing parameters to `syscall` instruction described in
    ; https://en.wikibooks.org/wiki/X86_Assembly/Interfacing_with_Linux#syscall
    syscall
    mov rax, 60 ; exit syscall (https://linux.die.net/man/2/exit)
    mov rdi, 0 ; exit code
    syscall
