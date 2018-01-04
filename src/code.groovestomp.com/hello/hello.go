package main

import "fmt"

func greeting(name string) bool {
	fmt.Printf("Hello %v\n", name)
	return true
}

func main() {
	name := "Aaron"
	greeted := greeting(name)
	if greeted {
		fmt.Println("Successfully greeted")
	} else {
		fmt.Println("Greeting failed")
	}
}
