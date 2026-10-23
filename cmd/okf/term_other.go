//go:build !(linux || darwin || freebsd)

package main

import "os"

func ttyWidth(*os.File) int { return 0 }
