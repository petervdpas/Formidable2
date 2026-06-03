// Package proc holds OS-process helpers shared by modules that shell out to
// external binaries (git, a user command, a version probe). Its job is to make
// child-process spawning behave the same regardless of host, the most visible
// case being Windows console windows.
package proc
