package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
)

func main() {
	outputName := flag.String("output", "", "output name")
	flag.Parse()

	data, err := os.ReadFile("./output.txt")
	if err != nil {
		log.Fatal(err)
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		parts := strings.Split(line, "=")
		if len(parts) != 2 {
			continue
		}
		if parts[0] == *outputName {
			fmt.Print(parts[1])
			break
		}
	}
}
