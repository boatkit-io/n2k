//go:build mage
// +build mage

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/magefile/mage/mg" // mg contains helpful utility functions, like Deps
	"github.com/magefile/mage/sh"
)

var Default = Build

var pgnGeneratedCodePath = filepath.Join("pkg", "n2k", "pgninfo_generated.go")

// Build builds the code
func Build() error {
	// If we haven't codegened yet, do that first
	if _, err := os.Stat(pgnGeneratedCodePath); os.IsNotExist(err) {
		mg.Deps(CodeGen)
	}

	mg.Deps(Dep)
	fmt.Println("Building...")
	return sh.RunV("go", "build", "-v", "-o", "bin/", "./cmd/...")
}

// Dep ensures that we've got our dependencies
func Dep() error {
	fmt.Println("Installing Deps...")
	if err := sh.RunV("go", "mod", "download"); err != nil {
		return err
	}

	return nil
}

func CodeGen() error {
	return sh.RunV("go", "run", "./cmd/pgngen/main.go", "./cmd/pgngen/deduper.go")
}

// Test runs the tests on this repository
func Test() error {
	if os.Getenv("CI") == "" {
		if err := sh.Run("golangci-lint", "run"); err != nil {
			return err
		}
	}

	return TestFast()
}

// TestFast runs the tests on this repository without golangci-lint, for fast cycles
func TestFast() error {
	return sh.RunV("go", "test", "-coverpkg=./...", "-coverprofile=c.out", "./...")
}

// Clean cleans up all build artifacts
func Clean() {
	fmt.Println("Cleaning...")
	os.RemoveAll("bin")
}
