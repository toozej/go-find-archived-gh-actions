// Package main provides the entry point for the go-find-archived-gh-actions application.
//
// This application serves as a template for Go projects, demonstrating
// best practices for CLI applications using cobra, logrus, and environment
// configuration management.
package main

import cmd "github.com/toozej/go-find-archived-gh-actions/cmd/go-find-archived-gh-actions"

// main is the entry point of the go-find-archived-gh-actions application.
// It delegates execution to the cmd package which handles all
// command-line interface functionality.
func main() {
	cmd.Execute()
}
