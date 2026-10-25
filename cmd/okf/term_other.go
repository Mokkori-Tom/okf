//go:build !(linux || darwin || freebsd)

package main

import "os"

func ttyWidth(*os.File) int { return 0 }

// isTTY cannot tell a terminal from other character devices here, so it
// trusts the character-device check in isTerminal.
func isTTY(*os.File) bool { return true }
