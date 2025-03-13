package main

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
)

// Given common default values for the command, generate it and execute it using gnokey
func TestGenerateAndExecuteCommand(t *testing.T) {
	mode := "addpkg"
	packageName := "test" + randomString(32)
	functionName := ""
	remote := "localhost:26657"
	keyName := "Dev"
	pkgDir := "."
	chainID := "dev"

	cmd := generateCommand(mode, packageName, functionName, remote, keyName, pkgDir, chainID)
	fmt.Println("DEBUG: ", cmd)

	// Expected output regex patterns
	heightPattern := regexp.MustCompile(`HEIGHT:\s+\d+`)
	txHashPattern := regexp.MustCompile(`TX HASH:\s+[A-Za-z0-9+/=]+`)

	password := ""

	// Execute the command
	output, err := executeCommand(cmd, password)

	// If execution should not fail
	if err != nil {
		t.Errorf("Command execution failed: %v", err)
	}

	// Normalize whitespace and check static output parts
	expectedStaticParts := []string{
		"OK!",
		"GAS WANTED: 800000",
		"GAS USED:",
		"EVENTS:     []",
	}
	for _, part := range expectedStaticParts {
		if !strings.Contains(output, part) {
			t.Errorf("Expected output to contain %q but it was missing.\nActual output: %q", part, output)
		}
	}

	// Validate dynamic fields using regex
	if !heightPattern.MatchString(output) {
		t.Errorf("Expected output to contain HEIGHT with an integer, but got:\n%s", output)
	}
	if !txHashPattern.MatchString(output) {
		t.Errorf("Expected output to contain TX HASH with a base64 string, but got:\n%s", output)
	}
}

func TestRandomStringUnique(t *testing.T) {
	r1 := randomString(32)
	r2 := randomString(32)

	if r1 == r2 {
		t.Errorf("Random calls not uinmque")
	}
}
